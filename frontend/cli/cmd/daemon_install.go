package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	_ "embed"

	"connectrpc.com/connect"
	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/furisto/construct/frontend/cli/pkg/terminal"
	"github.com/furisto/construct/shared"
	"github.com/spf13/cobra"
)

const DefaultInstallDirectory = "/usr/local/bin"

//go:embed deployment/macos/http.xml
var macosHTTPTemplate string

//go:embed deployment/macos/unix.xml
var macosUnixTemplate string

//go:embed deployment/linux/construct-http.socket
var linuxHTTPSocketTemplate string

//go:embed deployment/linux/construct-unix.socket
var linuxUnixSocketTemplate string

//go:embed deployment/linux/construct.service
var linuxServiceTemplate string

type daemonInstallOptions struct {
	Force         bool
	Name          string
	AlwaysRunning bool
	HTTPAddress   string
	Quiet         bool
}

func NewDaemonInstallCmd() *cobra.Command {
	options := daemonInstallOptions{}
	cmd := &cobra.Command{
		Use:   "install [flags]",
		Short: "Install and enable the Construct daemon as a system service",
		Args:  cobra.NoArgs,
		Long:  `Install and enable the Construct daemon as a system service.

Installs the daemon using the appropriate service manager for your OS (e.g., launchd 
on macOS, systemd on Linux). The daemon is required for most construct operations.`,
		Example: `  # Install the daemon with default settings
  construct daemon install`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if options.Quiet {
				out = io.Discard
			}

			var socketType string
			if options.HTTPAddress != "" {
				socketType = "http"
			} else {
				socketType = "unix"
			}

			endpointContext, err := installDaemon(cmd.Context(), out, socketType, options)
			if err != nil {
				return err
			}

			setupComplete, err := checkConnectionAndSetupStatus(cmd.Context(), out, *endpointContext)
			if err != nil {
				troubleshooting := buildTroubleshootingMessage(cmd.Context(), endpointContext)
				return fail.NewUserFacingError(fmt.Sprintf("Connection to daemon failed: %s", err), err, troubleshooting, "",
					[]string{"https://docs.construct.sh/daemon/troubleshooting"})
			}

			fmt.Fprintf(out, "%s Daemon installed successfully\n", terminal.SuccessSymbol)

			if setupComplete {
				cmd.Printf("%s Ready to use! Try 'construct new' to start a conversation\n", terminal.ContinueSymbol)
			} else {
				cmd.Printf("%s Next: Create a model provider with 'construct modelprovider create'\n", terminal.ContinueSymbol)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Force install the daemon")
	cmd.Flags().BoolVarP(&options.AlwaysRunning, "always-running", "", false, "Run the daemon continuously instead of using socket activation")
	cmd.Flags().StringVarP(&options.HTTPAddress, "listen-http", "", "", "HTTP address to listen on")
	cmd.Flags().BoolVarP(&options.Quiet, "quiet", "q", false, "Silent installation")
	cmd.Flags().StringVarP(&options.Name, "name", "n", "default", "Name of the daemon (used for socket activation and context)")

	return cmd
}

func installDaemon(ctx context.Context, out io.Writer, socketType string, options daemonInstallOptions) (*api.EndpointContext, error) {
	execPath, err := executableInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable info: %w", err)
	}

	runtimeInfo := getRuntimeInfo(ctx)
	switch runtimeInfo.GOOS() {
	case "darwin":
		err = installLaunchdService(ctx, out, socketType, execPath, options)
	case "linux":
		err = installSystemdService(ctx, out, socketType, execPath, options)
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtimeInfo.GOOS())
	}

	if err != nil {
		return nil, err
	}

	endpointContext, err := createOrUpdateContext(ctx, out, socketType, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create context: %w", err)
	}

	return endpointContext, nil
}

type serviceTemplateData struct {
	ExecPath    string
	Name        string
	HTTPAddress string
	KeepAlive   bool
}

func installLaunchdService(ctx context.Context, out io.Writer, socketType, execPath string, options daemonInstallOptions) error {
	fs := getFileSystem(ctx)
	command := getCommandRunner(ctx)

	userInfo := getUserInfo(ctx)
	homeDir, err := userInfo.ConstructDataDir()
	if err != nil {
		return fail.HandleError(err)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := fs.MkdirAll(launchAgentsDir, 0755); err != nil {
		if os.IsPermission(err) {
			return fail.NewPermissionError(launchAgentsDir, err)
		}
		return fmt.Errorf("failed to create LaunchAgents directory %s: %w", launchAgentsDir, err)
	}

	var macosTemplate string
	switch socketType {
	case "http":
		macosTemplate = macosHTTPTemplate
	case "unix":
		macosTemplate = macosUnixTemplate
	default:
		return fmt.Errorf("invalid socket type: %s", socketType)
	}
	filename := fmt.Sprintf("construct-%s.plist", options.Name)

	content, err := parseServiceTemplate(ctx, options, execPath, macosTemplate)
	if err != nil {
		return fail.HandleError(err)
	}

	plistPath := filepath.Join(launchAgentsDir, filename)
	if !options.Force {
		if exists, _ := fs.Exists(plistPath); exists {
			return fail.NewAlreadyInstalledError(plistPath)
		}
	}

	if err := fs.WriteFile(plistPath, content, 0644); err != nil {
		if os.IsPermission(err) {
			return fail.NewPermissionError(plistPath, err)
		}
		return fmt.Errorf("failed to write plist file to %s: %w", plistPath, err)
	}
	fmt.Fprintf(out, " %s Service file written to %s\n", terminal.SuccessSymbol, plistPath)

	userID := userInfo.UserID()
	if output, err := command.Run(ctx, "launchctl", "bootstrap", "gui/"+userID, plistPath); err != nil {
		return fail.NewCommandError("launchctl bootstrap", err, output, "gui/"+userID, plistPath)
	}

	fmt.Fprintf(out, " %s Launchd service loaded\n", terminal.SuccessSymbol)
	return nil
}

func installSystemdService(ctx context.Context, out io.Writer, socketType, execPath string, options daemonInstallOptions) error {
	fs := getFileSystem(ctx)
	command := getCommandRunner(ctx)

	var systemdTemplate string

	switch socketType {
	case "http":
		systemdTemplate = linuxHTTPSocketTemplate
	case "unix":
		systemdTemplate = linuxUnixSocketTemplate
	default:
		return fmt.Errorf("invalid socket type: %s", socketType)
	}

	socketPath := "/etc/systemd/system/construct.socket"
	servicePath := "/etc/systemd/system/construct.service"

	if !options.Force {
		if exists, _ := fs.Exists(socketPath); exists {
			return fail.NewAlreadyInstalledError(socketPath)
		}

		if exists, _ := fs.Exists(servicePath); exists {
			return fail.NewAlreadyInstalledError(servicePath)
		}
	}

	socketContent, err := parseServiceTemplate(ctx, options, execPath, systemdTemplate)
	if err != nil {
		return fail.HandleError(err)
	}

	if err := fs.WriteFile(socketPath, socketContent, 0644); err != nil {
		if os.IsPermission(err) {
			return fail.NewPermissionError(socketPath, err)
		}
		return fmt.Errorf("failed to write socket file: %w", err)
	}
	fmt.Fprintf(out, "%s Socket file written to %s\n", terminal.SuccessSymbol, socketPath)

	serviceContent, err := parseServiceTemplate(ctx, options, execPath, linuxServiceTemplate)
	if err != nil {
		return fail.HandleError(err)
	}

	if err := fs.WriteFile(servicePath, serviceContent, 0644); err != nil {
		if os.IsPermission(err) {
			return fail.NewPermissionError(servicePath, err)
		}
		return fmt.Errorf("failed to write service file: %w", err)
	}
	fmt.Fprintf(out, "%s Service file written to %s\n", terminal.SuccessSymbol, servicePath)

	if output, err := command.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return fail.NewCommandError("systemctl daemon-reload", err, output)
	}
	fmt.Fprintf(out, "%s Systemd daemon reloaded\n", terminal.SuccessSymbol)

	if output, err := command.Run(ctx, "systemctl", "enable", "construct.socket"); err != nil {
		return fail.NewCommandError("systemctl enable construct.socket", err, output)
	}
	fmt.Fprintf(out, "%s Socket enabled\n", terminal.SuccessSymbol)

	return nil
}

func executableInfo() (execPath string, err error) {
	execPath, err = os.Executable()
	if err != nil {
		return "", fail.HandleError(err)
	}

	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		// If symlink resolution fails, use original path
		realPath = execPath
	}

	return realPath, nil
}

func parseServiceTemplate(ctx context.Context, options daemonInstallOptions, execPath string, serviceTemplate string) ([]byte, error) {
	tmpl, err := template.New("daemon-install").Parse(serviceTemplate)
	if err != nil {
		return nil, fail.HandleError(err)
	}

	var content bytes.Buffer
	err = tmpl.Execute(&content, serviceTemplateData{
		ExecPath:    execPath,
		Name:        options.Name,
		HTTPAddress: options.HTTPAddress,
		KeepAlive:   options.AlwaysRunning,
	})
	if err != nil {
		return nil, fail.HandleError(err)
	}

	return content.Bytes(), nil
}

func createOrUpdateContext(ctx context.Context, out io.Writer, socketType string, options daemonInstallOptions) (*api.EndpointContext, error) {
	fs := getFileSystem(ctx)
	userInfo := getUserInfo(ctx)

	var address string
	switch socketType {
	case "http":
		address = options.HTTPAddress
	case "unix":
		address = fmt.Sprintf("/tmp/construct-%s.sock", options.Name)
	default:
		return nil, fmt.Errorf("invalid socket type: %s", socketType)
	}

	contextManager := shared.NewContextManager(fs, userInfo)
	exists, err := contextManager.UpsertContext(options.Name, socketType, address, true)
	if err != nil {
		return nil, fail.HandleError(err)
	}

	if exists {
		fmt.Fprintf(out, " %s Context '%s' updated\n", terminal.SuccessSymbol, options.Name)
	} else {
		fmt.Fprintf(out, " %s Context '%s' created\n", terminal.SuccessSymbol, options.Name)
	}

	endpointContexts, err := contextManager.LoadContext()
	if err != nil {
		return nil, fail.HandleError(err)
	}

	endpointContext, _ := endpointContexts.Current()
	return &endpointContext, nil
}

func checkConnectionAndSetupStatus(ctx context.Context, out io.Writer, endpoint api.EndpointContext) (bool, error) {
	canConnect, err := terminal.SpinnerFunc(
		out,
		"Checking connection to daemon",
		func() (bool, error) {
			client := api.NewClient(endpoint)

			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			resp, err := client.ModelProvider().ListModelProviders(ctx, &connect.Request[v1.ListModelProvidersRequest]{
				Msg: &v1.ListModelProvidersRequest{},
			})

			if err != nil {
				return false, fmt.Errorf("failed to check connection: %w", err)
			}

			return len(resp.Msg.ModelProviders) != 0, nil
		},
		terminal.WithSuccessMsg("Daemon is responding to requests"),
		terminal.WithErrorMsg("Failed to connect to daemon"),
	)
	return canConnect, err
}

func buildTroubleshootingMessage(ctx context.Context, endpointContext *api.EndpointContext) fail.Troubleshooting {
	var solutions []string
	runtimeInfo := getRuntimeInfo(ctx)

	switch runtimeInfo.GOOS() {
	case "darwin":
		solutions = append(solutions, "Check if the daemon service is running:\n   launchctl list | grep construct")

		solutions = append(solutions, "Check service status and logs:\n   # List all construct services:\n   launchctl list | grep construct\n   # Check specific service (replace 'default' with your service name if different):\n   launchctl print gui/$(id -u)/construct-default\n   # View recent logs:\n   log show --predicate 'process == \"construct\"' --last 5m")

		solutions = append(solutions, "Try manually starting the service:\n   # Replace 'default' with your service name if different:\n   launchctl kickstart -k gui/$(id -u)/construct-default")

	case "linux":
		solutions = append(solutions, "Check if the daemon socket is active:\n   systemctl --user status construct.socket\n   systemctl --user status construct.service")

		solutions = append(solutions, "Check service logs:\n   journalctl --user -u construct.service --no-pager -n 20\n   journalctl --user -u construct.socket --no-pager -n 20")

		solutions = append(solutions, "Try manually starting the socket:\n   systemctl --user start construct.socket\n   systemctl --user start construct.service")
	}

	// Verify the daemon endpoint
	var endpointSolution strings.Builder
	endpointSolution.WriteString("Verify the daemon endpoint:\n")
	endpointSolution.WriteString(fmt.Sprintf("   Address: %s\n", endpointContext.Address))
	endpointSolution.WriteString(fmt.Sprintf("   Type: %s\n", endpointContext.Kind))
	if endpointContext.Kind == "unix" {
		endpointSolution.WriteString("   Check if socket file exists and has correct permissions:\n")
		if strings.HasPrefix(endpointContext.Address, "unix://") {
			socketPath := strings.TrimPrefix(endpointContext.Address, "unix://")
			endpointSolution.WriteString(fmt.Sprintf("   ls -la %s", socketPath))
		} else {
			endpointSolution.WriteString("   ls -la /tmp/construct.sock")
		}
	} else {
		endpointSolution.WriteString("   Check if the HTTP port is accessible and not blocked by firewall:\n")
		if strings.Contains(endpointContext.Address, ":") {
			endpointSolution.WriteString(fmt.Sprintf("   curl -v %s/health || nc -zv %s", endpointContext.Address, endpointContext.Address))
		} else {
			endpointSolution.WriteString("   Check firewall settings and port availability")
		}
	}
	solutions = append(solutions, endpointSolution.String())

	// Check for permission issues
	var permissionSolution strings.Builder
	permissionSolution.WriteString("Check for permission issues:\n")
	if runtimeInfo.GOOS() == "darwin" {
		permissionSolution.WriteString("   # Check if plist files exist:\n")
		permissionSolution.WriteString("   ls -la ~/Library/LaunchAgents/construct-*.plist")
	} else if runtimeInfo.GOOS() == "linux" {
		permissionSolution.WriteString("   # Check if systemd files exist:\n")
		permissionSolution.WriteString("   ls -la /etc/systemd/system/construct.*")
	}
	solutions = append(solutions, permissionSolution.String())

	// Try reinstalling the daemon
	var reinstallSolution strings.Builder
	reinstallSolution.WriteString("Try reinstalling the daemon:\n")
	reinstallSolution.WriteString("   construct daemon uninstall\n")
	reinstallSolution.WriteString("   construct daemon install")
	if endpointContext.Kind == "http" {
		reinstallSolution.WriteString(" --listen-http " + endpointContext.Address)
	}
	solutions = append(solutions, reinstallSolution.String())

	// For additional help
	solutions = append(solutions, "For additional help:\n   - Check if the construct binary is accessible and executable\n   - Verify system resources (disk space, memory)\n   - Run 'construct daemon run' manually to see direct error output")

	return fail.Troubleshooting{
		Format:    fail.UserFacingSolutionFormatMultiline,
		Solutions: solutions,
	}
}
