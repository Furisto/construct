package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "embed"

	"github.com/spf13/cobra"
)

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
		Example: `  # Install daemon with HTTP socket activation
  construct daemon install

  # Install daemon with Unix socket activation
  construct daemon install --unix

  # Force install (overwrite existing installation)
  construct daemon install http --force

  # Install daemon with Unix socket activation and force install
  construct daemon install unix --force --always-running`,
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

			err := installDaemon(cmd.Context(), out, socketType, options)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ Daemon installed successfully\n")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Force install the daemon")
	cmd.Flags().BoolVarP(&options.AlwaysRunning, "always-running", "", false, "Run the daemon continuously instead of using socket activation")
	cmd.Flags().StringVarP(&options.HTTPAddress, "listen-http", "", "8080", "HTTP address to listen on")
	cmd.Flags().BoolVarP(&options.Quiet, "quiet", "q", false, "Quiet mode")

	return cmd
}

func installDaemon(ctx context.Context, out io.Writer, socketType string, options daemonInstallOptions) error {
	execPath, _, err := getExecutableInfo()
	if err != nil {
		return fmt.Errorf("failed to get executable info: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchdService(ctx, out, socketType, execPath, options)
	case "linux":
		return installSystemdService(ctx, out, socketType, execPath, options)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func installLaunchdService(ctx context.Context, out io.Writer, socketType, execPath string, options daemonInstallOptions) error {
	fs := getFileSystem(ctx)
	command := getCommandRunner(ctx)

	var template string
	var filename string

	switch socketType {
	case "http":
		template = macosHTTPTemplate
		filename = "construct-http.plist"
	case "unix":
		template = macosUnixTemplate
		filename = "construct-unix.plist"
	default:
		return fmt.Errorf("invalid socket type: %s", socketType)
	}

	content := strings.ReplaceAll(template, "{{EXEC_PATH}}", execPath)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := fs.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	plistPath := filepath.Join(launchAgentsDir, filename)
	if !options.Force {
		if _, err := fs.Stat(plistPath); err == nil {
			return fmt.Errorf("daemon already installed at %s", plistPath)
		}
	}

	if err := fs.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}
	fmt.Fprintf(out, "✓ Service file written to %s\n", plistPath)

	if output, err := command.Run(ctx, "launchctl", "load", plistPath); err != nil {
		return fmt.Errorf("failed to load daemon: %w\nOutput: %s", err, output)
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

func getExecutableInfo() (execPath, execDir string, err error) {
	execPath, err = os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("failed to get executable path: %w", err)
	}

	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		// If symlink resolution fails, use original path
		realPath = execPath
	}

	return realPath, filepath.Dir(realPath), nil
}
