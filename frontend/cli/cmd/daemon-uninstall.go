package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

type daemonUninstallOptions struct {
	SkipConfirm bool
	Quiet       bool
}

func NewDaemonUninstallCmd() *cobra.Command {
	options := daemonUninstallOptions{}
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the Construct daemon",
		Args:  cobra.NoArgs,
		Example: `  # Uninstall daemon with confirmation prompt
  construct daemon uninstall

  # Uninstall daemon without confirmation
  construct daemon uninstall -y`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.Quiet {
				cmd.SetOut(io.Discard)
			}

			return uninstallDaemon(cmd.Context(), cmd, options)
		},
	}

	cmd.Flags().BoolVarP(&options.SkipConfirm, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVarP(&options.Quiet, "quiet", "q", false, "Quiet mode")

	return cmd
}

type installedService struct {
	ServiceType string
	SocketType  string
	Files       []string
}

func uninstallDaemon(ctx context.Context, cmd *cobra.Command, options daemonUninstallOptions) error {
	fs := getFileSystem(ctx)
	command := getCommandRunner(ctx)
	stdin := cmd.InOrStdin()
	stdout := cmd.OutOrStdout()

	installedServices, err := detectInstalledServices(fs)
	if err != nil {
		return fmt.Errorf("failed to detect installed services: %w", err)
	}

	if len(installedServices) == 0 {
		fmt.Fprintf(stdout, "✓ Construct daemon service not found. Nothing to do.\n")
		return nil
	}

	if !options.SkipConfirm {
		fmt.Fprintf(stdout, "The following actions will be performed to uninstall the Construct daemon:\n")
		for i, service := range installedServices {
			fmt.Fprintf(stdout, "  %d. Stop the running service: '%s'\n", i+1, getServiceName(service))
			fmt.Fprintf(stdout, "     Disable the service from starting on boot\n")
			fmt.Fprintf(stdout, "     Remove the following system files:\n")
			for _, file := range service.Files {
				fmt.Fprintf(stdout, "       - %s\n", file)
			}
		}
		fmt.Fprintf(stdout, "\n")

		if !confirm(stdin, stdout, "Are you sure you want to continue?") {
			fmt.Fprintf(stdout, "✗ Uninstall cancelled.\n")
			return nil
		}
	}

	for _, service := range installedServices {
		err := uninstallService(ctx, stdout, fs, command, service)
		if err != nil {
			return fmt.Errorf("failed to uninstall %s service: %w", service.ServiceType, err)
		}
	}

	fmt.Fprintf(stdout, "✓ Daemon uninstalled successfully\n")
	return nil
}

func detectInstalledServices(fs *afero.Afero) ([]installedService, error) {
	var services []installedService

	switch runtime.GOOS {
	case "darwin":
		macosServices, err := detectMacOSServices(fs)
		if err != nil {
			return nil, err
		}
		services = append(services, macosServices...)
	case "linux":
		linuxServices, err := detectLinuxServices(fs)
		if err != nil {
			return nil, err
		}
		services = append(services, linuxServices...)
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return services, nil
}

func detectMacOSServices(fs *afero.Afero) ([]installedService, error) {
	var services []installedService

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")

	httpPlistPath := filepath.Join(launchAgentsDir, "construct-http.plist")
	if exists, _ := fs.Exists(httpPlistPath); exists {
		services = append(services, installedService{
			ServiceType: "launchd",
			SocketType:  "http",
			Files:       []string{httpPlistPath},
		})
	}

	unixPlistPath := filepath.Join(launchAgentsDir, "construct-unix.plist")
	if exists, _ := fs.Exists(unixPlistPath); exists {
		services = append(services, installedService{
			ServiceType: "launchd",
			SocketType:  "unix",
			Files:       []string{unixPlistPath},
		})
	}

	return services, nil
}

func detectLinuxServices(fs *afero.Afero) ([]installedService, error) {
	var services []installedService

	socketPath := "/etc/systemd/system/construct.socket"
	servicePath := "/etc/systemd/system/construct.service"

	var files []string
	if exists, _ := fs.Exists(socketPath); exists {
		files = append(files, socketPath)
	}
	if exists, _ := fs.Exists(servicePath); exists {
		files = append(files, servicePath)
	}

	if len(files) > 0 {
		services = append(services, installedService{
			ServiceType: "systemd",
			SocketType:  "unknown", // Could be http or unix, but we don't differentiate in removal
			Files:       files,
		})
	}

	return services, nil
}

func getServiceName(service installedService) string {
	switch service.ServiceType {
	case "launchd":
		return fmt.Sprintf("construct-%s.plist", service.SocketType)
	case "systemd":
		return "construct.socket"
	default:
		return "unknown"
	}
}

func uninstallService(ctx context.Context, out io.Writer, fs *afero.Afero, command CommandRunner, service installedService) error {
	switch service.ServiceType {
	case "launchd":
		return uninstallLaunchdService(ctx, out, fs, command, service)
	case "systemd":
		return uninstallSystemdService(ctx, out, fs, command, service)
	default:
		return fmt.Errorf("unsupported service type: %s", service.ServiceType)
	}
}

func uninstallLaunchdService(ctx context.Context, out io.Writer, fs *afero.Afero, command CommandRunner, service installedService) error {
	for _, plistPath := range service.Files {
		// Try to unload the service (ignore errors if not loaded)
		_, _ = command.Run(ctx, "launchctl", "unload", plistPath)
		fmt.Fprintf(out, "✓ Launchd service unloaded\n")

		// Remove the plist file
		err := fs.Remove(plistPath)
		if err != nil {
			return fmt.Errorf("failed to remove plist file %s: %w", plistPath, err)
		}
		fmt.Fprintf(out, "✓ Service file removed: %s\n", plistPath)
	}

	return nil
}

func uninstallSystemdService(ctx context.Context, out io.Writer, fs *afero.Afero, command CommandRunner, service installedService) error {
	_, _ = command.Run(ctx, "systemctl", "stop", "construct.socket")
	_, _ = command.Run(ctx, "systemctl", "disable", "construct.socket")
	fmt.Fprintf(out, "✓ Systemd service stopped and disabled\n")

	for _, filePath := range service.Files {
		err := fs.Remove(filePath)
		if err != nil {
			return fmt.Errorf("failed to remove service file %s: %w", filePath, err)
		}
		fmt.Fprintf(out, "✓ Service file removed: %s\n", filePath)
	}

	_, _ = command.Run(ctx, "systemctl", "daemon-reload")
	fmt.Fprintf(out, "✓ Systemd daemon reloaded\n")

	return nil
}
