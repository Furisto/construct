package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	_ "embed"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/client"
	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/furisto/construct/frontend/cli/pkg/terminal"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
		Use:   "install",
		Short: "Install the daemon",
		Args:  cobra.NoArgs,
		Example: `  # Install daemon with Unix socket activation
  construct daemon install

  # Install daemon with HTTP socket activation
  construct daemon install --listen-http 127.0.0.1:8080

  # Force install (overwrite existing installation)
  construct daemon install --force

  # Install daemon with custom name and run continuously
  construct daemon install --name production --listen-http :8080 --always-running`,
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

			nextSteps, err := checkConnectionAndShowNextSteps(cmd.Context(), out, *endpointContext)
			if err != nil {
				troubleshootingMsg := buildTroubleshootingMessage(endpointContext)
				return fail.NewUserFacingError(fmt.Sprintf("Connection to daemon failed: %s", err), err, []string{troubleshootingMsg}, "",
					[]string{"https://docs.construct.sh/daemon/troubleshooting"})
			}

			fmt.Fprintf(out, "%s Daemon installed successfully\n", terminal.SuccessSymbol)
			fmt.Fprintf(out, "%s\n", nextSteps)

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

func installDaemon(ctx context.Context, out io.Writer, socketType string, options daemonInstallOptions) (*client.EndpointContext, error) {
	execPath, err := executableInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable info: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		err = installLaunchdService(ctx, out, socketType, execPath, options)
	case "linux":
		err = installSystemdService(ctx, out, socketType, execPath, options)
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
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

	tmpl, err := template.New("daemon-install").Parse(macosTemplate)
	if err != nil {
		return fail.EnhanceError(err, nil)
	}

	var content bytes.Buffer
	err = tmpl.Execute(&content, serviceTemplateData{
		ExecPath:    execPath,
		Name:        options.Name,
		HTTPAddress: options.HTTPAddress,
		KeepAlive:   options.AlwaysRunning,
	})
	if err != nil {
		return fail.EnhanceError(err, nil)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fail.EnhanceError(err, nil)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := fs.MkdirAll(launchAgentsDir, 0755); err != nil {
		if os.IsPermission(err) {
			return fail.NewPermissionError(launchAgentsDir, err)
		}
		return fmt.Errorf("failed to create LaunchAgents directory %s: %w", launchAgentsDir, err)
	}

	plistPath := filepath.Join(launchAgentsDir, filename)
	if !options.Force {
		if exists, _ := fs.Exists(plistPath); exists {
			return fail.NewAlreadyInstalledError(plistPath)
		}
	}

	if err := fs.WriteFile(plistPath, content.Bytes(), 0644); err != nil {
		if os.IsPermission(err) {
			return fail.NewPermissionError(plistPath, err)
		}
		return fmt.Errorf("failed to write plist file to %s: %w", plistPath, err)
	}
	fmt.Fprintf(out, "%s Service file written to %s\n", terminal.SuccessSymbol, plistPath)

	if output, err := command.Run(ctx, "launchctl", "bootstrap", "gui/"+getUserID(), plistPath); err != nil {
		return fail.NewCommandError("launchctl bootstrap", err, output, "gui/"+getUserID(), plistPath)
	}

	fmt.Fprintf(out, "%s Launchd service loaded\n", terminal.SuccessSymbol)
	return nil
}

func installSystemdService(ctx context.Context, out io.Writer, socketType, execPath string, options daemonInstallOptions) error {
	fs := getFileSystem(ctx)
	command := getCommandRunner(ctx)

	var socketTemplate string

	switch socketType {
	case "http":
		socketTemplate = linuxHTTPSocketTemplate
	case "unix":
		socketTemplate = linuxUnixSocketTemplate
	default:
		return fmt.Errorf("invalid socket type: %s", socketType)
	}

	serviceContent := strings.ReplaceAll(linuxServiceTemplate, "{{EXEC_PATH}}", execPath)

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

	if err := fs.WriteFile(socketPath, []byte(socketTemplate), 0644); err != nil {
		if os.IsPermission(err) {
			return fail.NewPermissionError(socketPath, err)
		}
		return fmt.Errorf("failed to write socket file: %w", err)
	}
	fmt.Fprintf(out, "%s Socket file written to %s\n", terminal.SuccessSymbol, socketPath)

	if err := fs.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
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
		return "", fail.EnhanceError(err, nil)
	}

	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		// If symlink resolution fails, use original path
		realPath = execPath
	}

	return realPath, nil
}

func createOrUpdateContext(ctx context.Context, out io.Writer, socketType string, options daemonInstallOptions) (*client.EndpointContext, error) {
	fs := getFileSystem(ctx)

	constructDir, err := ConstructUserDir()
	if err != nil {
		return nil, fail.EnhanceError(err, nil)
	}

	err = fs.MkdirAll(constructDir, 0755)
	if err != nil {
		return nil, fail.EnhanceError(err, map[string]interface{}{
			"path": constructDir,
		})
	}
	contextFile := filepath.Join(constructDir, "context.yaml")

	var address string
	switch socketType {
	case "http":
		address = options.HTTPAddress
	case "unix":
		address = fmt.Sprintf("unix:///tmp/construct-%s.sock", options.Name)
	default:
		return nil, fmt.Errorf("invalid socket type: %s", socketType)
	}

	var endpointContexts client.EndpointContexts
	exists, err := fs.Exists(contextFile)
	if err != nil {
		return nil, fail.EnhanceError(err, map[string]interface{}{
			"path": contextFile,
		})
	}

	if exists {
		content, err := fs.ReadFile(contextFile)
		if err != nil {
			return nil, fail.EnhanceError(err, map[string]interface{}{
				"path": contextFile,
			})
		}
		err = yaml.Unmarshal(content, &endpointContexts)
		if err != nil {
			return nil, fail.EnhanceError(err, map[string]interface{}{
				"path": contextFile,
			})
		}
	}

	contextName := options.Name
	if endpointContexts.Contexts == nil {
		endpointContexts.Contexts = make(map[string]client.EndpointContext)
	}

	endpointContexts.Contexts[contextName] = client.EndpointContext{
		Address: address,
		Type:    socketType,
	}

	endpointContexts.Current = contextName

	content, err := yaml.Marshal(&endpointContexts)
	if err != nil {
		return nil, fail.EnhanceError(err, nil)
	}

	err = fs.WriteFile(contextFile, content, 0644)
	if err != nil {
		return nil, fail.EnhanceError(err, map[string]interface{}{
			"path": contextFile,
		})
	}

	if exists {
		fmt.Fprintf(out, "%s Context '%s' updated\n", terminal.SuccessSymbol, contextName)
	} else {
		fmt.Fprintf(out, "%s Context '%s' created\n", terminal.SuccessSymbol, contextName)
	}

	return client.Ptr(endpointContexts.Contexts[contextName]), nil
}

func checkConnectionAndShowNextSteps(ctx context.Context, out io.Writer, endpoint client.EndpointContext) (nextSteps string, err error) {
	err = terminal.SpinnerFuncWithCustomCompletion(
		out,
		"Checking connection to daemon",
		fmt.Sprintf("%s Checking connection to daemon", terminal.SuccessSymbol),
		fmt.Sprintf("%s Checking connection to daemon", terminal.SmallErrorSymbol),
		func() error {
			client := api.NewClient(endpoint)
			time.Sleep(5 * time.Second)

			resp, err := client.ModelProvider().ListModelProviders(ctx, &connect.Request[v1.ListModelProvidersRequest]{
				Msg: &v1.ListModelProvidersRequest{},
			})

			if err != nil {
				return fmt.Errorf("failed to check connection: %w", err)
			}

			if len(resp.Msg.ModelProviders) == 0 {
				nextSteps = fmt.Sprintf("%s Next: Create a model provider with 'construct modelprovider create'", terminal.InfoSymbol)
			} else {
				nextSteps = fmt.Sprintf("%s Ready to use! Try 'construct new' to start a conversation", terminal.ActionSymbol)
			}

			return nil
		},
	)
	return nextSteps, err
}

func buildTroubleshootingMessage(endpointContext *client.EndpointContext) string {
	var msg strings.Builder

	msg.WriteString("Troubleshooting steps:\n")

	switch runtime.GOOS {
	case "darwin":
		msg.WriteString("1. Check if the daemon service is running:\n")
		msg.WriteString("   launchctl list | grep construct\n\n")

		msg.WriteString("2. Check service status and logs:\n")
		msg.WriteString("   # List all construct services:\n")
		msg.WriteString("   launchctl list | grep construct\n")
		msg.WriteString("   # Check specific service (replace 'default' with your service name if different):\n")
		msg.WriteString("   launchctl print gui/$(id -u)/construct-default\n")
		msg.WriteString("   # View recent logs:\n")
		msg.WriteString("   log show --predicate 'process == \"construct\"' --last 5m\n\n")

		msg.WriteString("3. Try manually starting the service:\n")
		msg.WriteString("   # Replace 'default' with your service name if different:\n")
		msg.WriteString("   launchctl kickstart -k gui/$(id -u)/construct-default\n\n")

	case "linux":
		msg.WriteString("1. Check if the daemon socket is active:\n")
		msg.WriteString("   systemctl --user status construct.socket\n")
		msg.WriteString("   systemctl --user status construct.service\n\n")

		msg.WriteString("2. Check service logs:\n")
		msg.WriteString("   journalctl --user -u construct.service --no-pager -n 20\n")
		msg.WriteString("   journalctl --user -u construct.socket --no-pager -n 20\n\n")

		msg.WriteString("3. Try manually starting the socket:\n")
		msg.WriteString("   systemctl --user start construct.socket\n")
		msg.WriteString("   systemctl --user start construct.service\n\n")
	}

	msg.WriteString("4. Verify the daemon endpoint:\n")
	msg.WriteString(fmt.Sprintf("   Address: %s\n", endpointContext.Address))
	msg.WriteString(fmt.Sprintf("   Type: %s\n", endpointContext.Type))
	if endpointContext.Type == "unix" {
		msg.WriteString("   Check if socket file exists and has correct permissions:\n")
		if strings.HasPrefix(endpointContext.Address, "unix://") {
			socketPath := strings.TrimPrefix(endpointContext.Address, "unix://")
			msg.WriteString(fmt.Sprintf("   ls -la %s\n\n", socketPath))
		} else {
			msg.WriteString("   ls -la /tmp/construct.sock\n\n")
		}
	} else {
		msg.WriteString("   Check if the HTTP port is accessible and not blocked by firewall:\n")
		if strings.Contains(endpointContext.Address, ":") {
			msg.WriteString(fmt.Sprintf("   curl -v %s/health || nc -zv %s\n\n", endpointContext.Address, endpointContext.Address))
		} else {
			msg.WriteString("   Check firewall settings and port availability\n\n")
		}
	}

	msg.WriteString("5. Check for permission issues:\n")
	if runtime.GOOS == "darwin" {
		msg.WriteString("   # Check if plist files exist:\n")
		msg.WriteString("   ls -la ~/Library/LaunchAgents/construct-*.plist\n")
	} else if runtime.GOOS == "linux" {
		msg.WriteString("   # Check if systemd files exist:\n")
		msg.WriteString("   ls -la /etc/systemd/system/construct.*\n")
	}
	msg.WriteString("\n")

	msg.WriteString("6. Try reinstalling the daemon:\n")
	msg.WriteString("   construct daemon uninstall\n")
	msg.WriteString("   construct daemon install")
	if endpointContext.Type == "http" {
		msg.WriteString(" --listen-http " + endpointContext.Address)
	}
	msg.WriteString("\n\n")

	msg.WriteString("7. For additional help:\n")
	msg.WriteString("   - Check if the construct binary is accessible and executable\n")
	msg.WriteString("   - Verify system resources (disk space, memory)\n")
	msg.WriteString("   - Run 'construct daemon run' manually to see direct error output")
	msg.WriteString("\n")

	return msg.String()
}
