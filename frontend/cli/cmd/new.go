package cmd

import (
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/terminal"
)

var newOptions struct {
	Socket string
}

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Start a new conversation",
	Run: func(cmd *cobra.Command, args []string) {
		go func() {
			err := RunAgent(cmd.Context())
			if err != nil {
				slog.Error("failed to run agent", "error", err)
			}
		}()

		apiClient, err := api_client.NewClient(cmd.Context(), ":29333")
		if err != nil {
			return
		}

		resp, err := apiClient.Task().CreateTask(cmd.Context(), &connect.Request[v1.CreateTaskRequest]{
			Msg: &v1.CreateTaskRequest{
				// Agent: "construct",
			},
		})
		if err != nil {
			slog.Error("failed to create task", "error", err)
			return
		}
		
		p := tea.NewProgram(terminal.NewModel(apiClient, resp.Msg.Task), tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running program: %v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
	newCmd.Flags().StringVarP(&newOptions.Socket, "socket", "s", "", "The socket to connect to")
}
