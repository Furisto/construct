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

type InstallError struct {
	Operation   string
	Cause       error
	UserMessage string
	Solutions   []string
	TechDetails string
	HelpURLs    []string
}

func (e *InstallError) Error() string {
	var msg strings.Builder

	msg.WriteString(fmt.Sprintf("✗ %s\n\n", e.UserMessage))

	if len(e.Solutions) > 0 {
		msg.WriteString("Try these solutions:\n")
		for i, solution := range e.Solutions {
			msg.WriteString(fmt.Sprintf("  %d. %s\n", i+1, solution))
		}
		msg.WriteString("\n")
	}

	if globalOptions.Verbose && e.TechDetails != "" {
		msg.WriteString(fmt.Sprintf("Technical details: %s\n\n", e.TechDetails))
	}

	if len(e.HelpURLs) > 0 {
		for _, url := range e.HelpURLs {
			msg.WriteString(fmt.Sprintf("→ %s\n", url))
		}
	}

	return msg.String()
}

func (e *InstallError) Unwrap() error {
	return e.Cause
}

func newPermissionError(operation, path string, err error) *InstallError {
	return &InstallError{
		Operation:   operation,
		Cause:       err,
		UserMessage: fmt.Sprintf("Permission denied accessing %s", path),
		Solutions: []string{
			"Check file permissions and ownership",
			"Ensure you have write access to the directory",
			"Try running with appropriate privileges if needed",
			"Verify the path exists and is accessible",
		},
		TechDetails: fmt.Sprintf("Failed to access %s: %v", path, err),
		HelpURLs: []string{
			"https://docs.construct.sh/daemon/troubleshooting#permission-errors",
			"https://github.com/furisto/construct/issues/new",
		},
	}
}

func newAlreadyInstalledError(path string) *InstallError {
	return &InstallError{
		Operation:   "check_existing_installation",
		Cause:       nil,
		UserMessage: "Construct daemon is already installed on this system",
		Solutions: []string{
			"Use '--force' flag to overwrite: construct daemon install --force",
			"Uninstall first: construct daemon uninstall && construct daemon install",
			"Use '--name' flag to create a separate daemon instance (advanced)",
		},
		TechDetails: fmt.Sprintf("Service file exists at: %s", path),
		HelpURLs: []string{
			"https://docs.construct.sh/daemon/troubleshooting#already-installed",
			"https://github.com/furisto/construct/issues/new",
		},
	}
}

func newSystemCommandError(command string, err error, output string) *InstallError {
	return &InstallError{
		Operation:   "system_command",
		Cause:       err,
		UserMessage: fmt.Sprintf("System command failed: %s", command),
		Solutions: []string{
			"Check if the required system service is running",
			"Verify you have permission to manage system services",
			"Check system logs for more details",
			"Try running the command manually to diagnose the issue",
		},
		TechDetails: fmt.Sprintf("Command '%s' failed: %v\nOutput: %s", command, err, output),
		HelpURLs: []string{
			"https://docs.construct.sh/daemon/troubleshooting#system-command-failed",
			"https://github.com/furisto/construct/issues/new",
		},
	}
}

func newConnectionError(address string, err error) *InstallError {
	var solutions []string

	if strings.Contains(err.Error(), "connection refused") {
		solutions = []string{
			"Wait a few seconds for the daemon to start, then try again",
			"Check if the daemon process is running",
			"Restart the daemon service",
			"Verify the address is correct and accessible",
		}
	} else if strings.Contains(err.Error(), "no such file") {
		solutions = []string{
			"Check if the socket file exists and has correct permissions",
			"Restart the daemon to recreate the socket",
			"Verify the socket path is correct",
		}
	} else {
		solutions = []string{
			"Check daemon logs for startup errors",
			"Verify the daemon binary is working",
			"Try reinstalling the daemon",
		}
	}

	return &InstallError{
		Operation:   "connection_check",
		Cause:       err,
		UserMessage: "Installation completed but cannot connect to the daemon",
		Solutions:   solutions,
		TechDetails: fmt.Sprintf("Connection failed to %s: %v", address, err),
		HelpURLs: []string{
			"https://docs.construct.sh/daemon/troubleshooting#connection-failed",
			"https://github.com/furisto/construct/issues/new",
		},
	}
}

func enhanceError(err error, operation string, context map[string]interface{}) error {
	if err == nil {
		return nil
	}

	if _, ok := err.(*InstallError); ok {
		return err
	}

	errStr := err.Error()

	if os.IsPermission(err) {
		if path, ok := context["path"].(string); ok {
			return newPermissionError(operation, path, err)
		}
	}

	if strings.Contains(errStr, "no such file or directory") {
		return &InstallError{
			Operation:   operation,
			Cause:       err,
			UserMessage: "Required file or directory not found",
			Solutions: []string{
				"Verify the path exists and is accessible",
				"Check if the parent directory exists",
				"Ensure the construct binary is properly installed",
			},
			TechDetails: errStr,
			HelpURLs: []string{
				"https://docs.construct.sh/daemon/troubleshooting#file-not-found",
				"https://github.com/furisto/construct/issues/new",
			},
		}
	}

	if strings.Contains(errStr, "address already in use") {
		return &InstallError{
			Operation:   operation,
			Cause:       err,
			UserMessage: "The network address is already in use by another process",
			Solutions: []string{
				"Choose a different port number",
				"Stop the process using this port",
				"Use Unix socket instead: construct daemon install",
			},
			TechDetails: errStr,
			HelpURLs: []string{
				"https://docs.construct.sh/daemon/troubleshooting#address-in-use",
				"https://github.com/furisto/construct/issues/new",
			},
		}
	}

	if strings.Contains(errStr, "operation not permitted") {
		return &InstallError{
			Operation:   operation,
			Cause:       err,
			UserMessage: "Operation not permitted - insufficient privileges",
			Solutions: []string{
				"Check if you have the necessary permissions",
				"Try running with appropriate privileges if needed",
				"Verify you can manage system services",
			},
			TechDetails: errStr,
			HelpURLs: []string{
				"https://docs.construct.sh/daemon/troubleshooting#operation-not-permitted",
				"https://github.com/furisto/construct/issues/new",
			},
		}
	}

	return err
}

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
	cmd.Flags().BoolVarP(&options.Quiet, "quiet", "q", false, "Silent installation")
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
		return enhanceError(err, "parse_service_template", nil)
	}

	var content bytes.Buffer
	err = tmpl.Execute(&content, serviceTemplateData{
		ExecPath:    execPath,
		Name:        options.Name,
		HTTPAddress: options.HTTPAddress,
		KeepAlive:   options.AlwaysRunning,
	})
	if err != nil {
		return enhanceError(err, "execute_service_template", nil)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return enhanceError(err, "get_home_directory", nil)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := fs.MkdirAll(launchAgentsDir, 0755); err != nil {
		if os.IsPermission(err) {
			return newPermissionError("create_directory", launchAgentsDir, err)
		}
		return fmt.Errorf("failed to create LaunchAgents directory %s: %w", launchAgentsDir, err)
	}

	plistPath := filepath.Join(launchAgentsDir, filename)
	if !options.Force {
		if exists, _ := fs.Exists(plistPath); exists {
			return newAlreadyInstalledError(plistPath)
		}
	}

	if err := fs.WriteFile(plistPath, content.Bytes(), 0644); err != nil {
		if os.IsPermission(err) {
			return newPermissionError("write_file", plistPath, err)
		}
		return fmt.Errorf("failed to write plist file to %s: %w", plistPath, err)
	}
	fmt.Fprintf(out, "✓ Service file written to %s\n", plistPath)

	if output, err := command.Run(ctx, "launchctl", "bootstrap", "gui/"+getUserID(), plistPath); err != nil {
		return newSystemCommandError("launchctl bootstrap", err, output)
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
			return newAlreadyInstalledError(socketPath)
		}

		if _, err := fs.Stat(servicePath); err == nil {
			return newAlreadyInstalledError(servicePath)
		}
	}

	if err := fs.WriteFile(socketPath, []byte(socketTemplate), 0644); err != nil {
		if os.IsPermission(err) {
			return newPermissionError("write_file", socketPath, err)
		}
		return fmt.Errorf("failed to write socket file: %w", err)
	}
	fmt.Fprintf(out, "✓ Socket file written to %s\n", socketPath)

	if err := fs.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		if os.IsPermission(err) {
			return newPermissionError("write_file", servicePath, err)
		}
		return fmt.Errorf("failed to write service file: %w", err)
	}
	fmt.Fprintf(out, "✓ Service file written to %s\n", servicePath)

	if output, err := command.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return newSystemCommandError("systemctl daemon-reload", err, output)
	}
	fmt.Fprintf(out, "✓ Systemd daemon reloaded\n")

	if output, err := command.Run(ctx, "systemctl", "enable", "construct.socket"); err != nil {
		return newSystemCommandError("systemctl enable construct.socket", err, output)
	}
	fmt.Fprintf(out, "✓ Socket enabled\n")

	return nil
}

func executableInfo() (execPath string, err error) {
	execPath, err = os.Executable()
	if err != nil {
		return "", enhanceError(err, "get_executable_path", nil)
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
		return EndpointContext{}, enhanceError(err, "get_construct_user_directory", nil)
	}

	err = fs.MkdirAll(constructDir, 0755)
	if err != nil {
		return EndpointContext{}, enhanceError(err, "create_construct_directory", map[string]interface{}{
			"path": constructDir,
		})
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
		return EndpointContext{}, enhanceError(err, "check_context_file", map[string]interface{}{
			"path": contextFile,
		})
	}

	if exists {
		content, err := fs.ReadFile(contextFile)
		if err != nil {
			return EndpointContext{}, enhanceError(err, "read_context_file", map[string]interface{}{
				"path": contextFile,
			})
		}
		err = yaml.Unmarshal(content, &endpointContexts)
		if err != nil {
			return EndpointContext{}, enhanceError(err, "parse_context_file", map[string]interface{}{
				"path": contextFile,
			})
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
		return EndpointContext{}, enhanceError(err, "marshal_context_data", nil)
	}

	err = fs.WriteFile(contextFile, content, 0644)
	if err != nil {
		return EndpointContext{}, enhanceError(err, "write_context_file", map[string]interface{}{
			"path": contextFile,
		})
	}

	if exists {
		fmt.Fprintf(out, "✓ Context '%s' updated\n", contextName)
	} else {
		fmt.Fprintf(out, "✓ Context '%s' created\n", contextName)
	}

	return endpointContexts.Contexts[contextName], nil
}
