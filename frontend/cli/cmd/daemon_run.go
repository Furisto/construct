package cmd

import (
	"fmt"
	"os"

	"entgo.io/ent/dialect"
	"github.com/furisto/construct/backend/agent"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/tool"
	"github.com/furisto/construct/shared/listener"
	"github.com/spf13/cobra"
)

type daemonRunOptions struct {
	HTTPAddress string
	UnixSocket  string
}

func NewDaemonRunCmd() *cobra.Command {
	options := daemonRunOptions{}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the API server as a persistent service",
		Long: `The "daemon" command allows you to run the construct server as a single, long-running
		  process. It supports different launch modes:
		
		  On macOS:
		  - If launched by launchd: uses HTTP address if provided, otherwise uses socket activation
		  - If not launched by launchd: uses provided HTTP address or Unix socket
		
		  On Linux:
		  - If launched by systemd: uses HTTP address if provided, otherwise uses socket activation  
		  - If not launched by systemd: uses provided HTTP address or Unix socket`,
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			memory, err := memory.Open(dialect.SQLite, "file:"+homeDir+"/.construct/construct.db?_fk=1&_journal=WAL&_busy_timeout=5000")
			if err != nil {
				return err
			}
			defer memory.Close()

			if err := memory.Schema.Create(cmd.Context()); err != nil {
				return err
			}

			encryption, err := getEncryptionClient()
			if err != nil {
				return err
			}

			provider, err := listener.DetectProvider(options.HTTPAddress, options.UnixSocket)
			if err != nil {
				return fmt.Errorf("failed to detect listener provider: %w", err)
			}

			listener, err := provider.Create()
			if err != nil {
				return fmt.Errorf("failed to create listener: %w", err)
			}

			runtime, err := agent.NewRuntime(
				memory,
				encryption,
				listener,
				agent.WithCodeActTools(
					tool.NewCreateFileTool(),
					tool.NewReadFileTool(),
					tool.NewEditFileTool(),
					tool.NewListFilesTool(),
					tool.NewGrepTool(),
					tool.NewExecuteCommandTool(),
					tool.NewPrintTool(),
				),
			)

			if err != nil {
				return err
			}

			return runtime.Run(cmd.Context())
		},
	}

	cmd.Flags().StringVar(&options.HTTPAddress, "listen-http", "", "The address to listen on for HTTP requests")
	cmd.Flags().StringVar(&options.UnixSocket, "listen-unix", "", "The path to listen on for Unix socket requests")

	return cmd
}
