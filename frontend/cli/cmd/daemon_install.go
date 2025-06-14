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

	_ "embed"

	"connectrpc.com/connect"
	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
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

			nextSteps, err := checkConnectionAndShowNextSteps(cmd.Context(), out, endpointContext)
			if err != nil {
				troubleshootingMsg := buildTroubleshootingMessage(endpointContext)
				return fmt.Errorf("connection to daemon failed: %w\n\n%s", err, troubleshootingMsg)
			}

			fmt.Fprintf(out, "✓ Daemon installed successfully\n")
			fmt.Fprintf(out, "%s\n", nextSteps)

			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Force install the daemon")
	cmd.Flags().BoolVarP(&options.AlwaysRunning, "always-running", "", false, "Run the daemon continuously instead of using socket activation")
	cmd.Flags().StringVarP(&options.HTTPAddress, "listen-http", "", "", "HTTP address to listen on")
	cmd.Flags().BoolVarP(&options.Quiet, "quiet", "q", false, "Quiet mode")
	cmd.Flags().StringVarP(&options.Name, "name", "n", "default", "Name of the daemon (used for socket activation and context)")

	return cmd
}

func installDaemon(ctx context.Context, out io.Writer, socketType string, options daemonInstallOptions) (*EndpointContext, error) {
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

	return &endpointContext, nil
}

func checkConnectionAndShowNextSteps(ctx context.Context, out io.Writer, endpointContext *EndpointContext) (nextSteps string, err error) {
	err = terminal.SpinnerFuncWithCustomCompletion(
		out,
		"Checking connection to daemon",
		"✓ Connection to daemon successful",
		"✗ Connection to daemon failed",
		func() error {
			client := api.NewClient(endpointContext.Address)

			resp, err := client.ModelProvider().ListModelProviders(ctx, &connect.Request[v1.ListModelProvidersRequest]{
				Msg: &v1.ListModelProvidersRequest{},
			})

			if err != nil {
				return fmt.Errorf("failed to check connection: %w", err)
			}

			if len(resp.Msg.ModelProviders) == 0 {
				nextSteps = "→ Next: Create a model provider with 'construct modelprovider create'"
			} else {
				nextSteps = "→ Ready to use! Try 'construct new' to start a conversation"
			}

			return nil
		},
	)
	return nextSteps, err
}

func buildTroubleshootingMessage(endpointContext *EndpointContext) string {
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

	return msg.String()
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
		return fmt.Errorf("failed to parse service template: %w", err)
	}

	var content bytes.Buffer
	err = tmpl.Execute(&content, serviceTemplateData{
		ExecPath:    execPath,
		Name:        options.Name,
		HTTPAddress: options.HTTPAddress,
		KeepAlive:   options.AlwaysRunning,
	})
	if err != nil {
		return fmt.Errorf("failed to execute service template: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := fs.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory %s: %w", launchAgentsDir, err)
	}

	plistPath := filepath.Join(launchAgentsDir, filename)
	if !options.Force {
		if exists, _ := fs.Exists(plistPath); exists {
			return fmt.Errorf("daemon already installed at %s, use --force to overwrite", plistPath)
		}
	}

	if err := fs.WriteFile(plistPath, content.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write plist file to %s: %w", plistPath, err)
	}
	fmt.Fprintf(out, "✓ Service file written to %s\n", plistPath)

	if output, err := command.Run(ctx, "launchctl", "bootstrap", "gui/"+getUserID(), plistPath); err != nil {
		return fmt.Errorf("failed to bootstrap daemon: %w\nOutput: %s", err, output)
	}

	fmt.Fprintf(out, "✓ Launchd service loaded\n")
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
		if _, err := fs.Stat(socketPath); err == nil {
			return fmt.Errorf("daemon socket already installed at %s", socketPath)
		}

		if _, err := fs.Stat(servicePath); err == nil {
			return fmt.Errorf("daemon service already installed at %s", servicePath)
		}
	}

	if err := fs.WriteFile(socketPath, []byte(socketTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write socket file: %w", err)
	}
	fmt.Fprintf(out, "✓ Socket file written to %s\n", socketPath)

	if err := fs.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}
	fmt.Fprintf(out, "✓ Service file written to %s\n", servicePath)

	if output, err := command.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w\nOutput: %s", err, output)
	}
	fmt.Fprintf(out, "✓ Systemd daemon reloaded\n")

	if output, err := command.Run(ctx, "systemctl", "enable", "construct.socket"); err != nil {
		return fmt.Errorf("failed to enable socket: %w\nOutput: %s", err, output)
	}
	fmt.Fprintf(out, "✓ Socket enabled\n")

	return nil
}

func executableInfo() (execPath string, err error) {
	execPath, err = os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		// If symlink resolution fails, use original path
		realPath = execPath
	}

	return realPath, nil
}

func createOrUpdateContext(ctx context.Context, out io.Writer, socketType string, options daemonInstallOptions) (EndpointContext, error) {
	fs := getFileSystem(ctx)

	constructDir, err := ConstructUserDir()
	if err != nil {
		return EndpointContext{}, fmt.Errorf("failed to get construct user directory: %w", err)
	}

	err = fs.MkdirAll(constructDir, 0755)
	if err != nil {
		return EndpointContext{}, fmt.Errorf("failed to create .construct directory: %w", err)
	}
	contextFile := filepath.Join(constructDir, "context.yaml")

	var address string
	switch socketType {
	case "http":
		address = options.HTTPAddress
	case "unix":
		address = "unix:///tmp/construct.sock"
	default:
		return EndpointContext{}, fmt.Errorf("invalid socket type: %s", socketType)
	}

	var endpointContexts EndpointContexts
	exists, err := fs.Exists(contextFile)
	if err != nil {
		return EndpointContext{}, fmt.Errorf("failed to check context file existence: %w", err)
	}

	if exists {
		content, err := fs.ReadFile(contextFile)
		if err != nil {
			return EndpointContext{}, fmt.Errorf("failed to read existing context file: %w", err)
		}
		err = yaml.Unmarshal(content, &endpointContexts)
		if err != nil {
			return EndpointContext{}, fmt.Errorf("failed to parse existing context file: %w", err)
		}
	}

	contextName := options.Name
	if endpointContexts.Contexts == nil {
		endpointContexts.Contexts = make(map[string]EndpointContext)
	}

	endpointContexts.Contexts[contextName] = EndpointContext{
		Address: address,
		Type:    socketType,
	}

	endpointContexts.Current = contextName

	content, err := yaml.Marshal(&endpointContexts)
	if err != nil {
		return EndpointContext{}, fmt.Errorf("failed to marshal context data: %w", err)
	}

	err = fs.WriteFile(contextFile, content, 0644)
	if err != nil {
		return EndpointContext{}, fmt.Errorf("failed to write context file: %w", err)
	}

	if exists {
		fmt.Fprintf(out, "✓ Context '%s' updated\n", contextName)
	} else {
		fmt.Fprintf(out, "✓ Context '%s' created\n", contextName)
	}

	return endpointContexts.Contexts[contextName], nil
}
