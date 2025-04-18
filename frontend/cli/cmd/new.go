package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"connectrpc.com/connect"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/spf13/cobra"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/terminal"
)

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Start a new conversation",
	Run: func(cmd *cobra.Command, args []string) {
		tempFile, err := os.CreateTemp("", "construct-new-*")
		if err != nil {
			slog.Error("failed to create temp file", "error", err)
			return
		}

		fmt.Println("Temp file created", tempFile.Name())

		tea.LogToFile(tempFile.Name(), "debug")

		slog.SetDefault(slog.New(slog.NewTextHandler(tempFile, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
		apiClient := getClient()

		resp, err := apiClient.Task().CreateTask(cmd.Context(), &connect.Request[v1.CreateTaskRequest]{
			Msg: &v1.CreateTaskRequest{
				AgentId: "2c341901-58bd-4ece-8967-1d28d6341c5d",
			},
		})

		if err != nil {
			slog.Error("failed to create task", "error", err)
			return
		}

		agentResp, err := apiClient.Agent().GetAgent(cmd.Context(), &connect.Request[v1.GetAgentRequest]{
			Msg: &v1.GetAgentRequest{
				Id: "2c341901-58bd-4ece-8967-1d28d6341c5d",
			},
		})
		if err != nil {
			slog.Error("failed to get agent", "error", err)
			return
		}

		p := tea.NewProgram(terminal.NewModel(cmd.Context(), apiClient, resp.Msg.Task, agentResp.Msg.Agent), tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running program: %v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
}
