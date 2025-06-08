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
		apiClient := getAPIClient(cmd.Context())

		agentResp, err := apiClient.Agent().ListAgents(cmd.Context(), &connect.Request[v1.ListAgentsRequest]{
			Msg: &v1.ListAgentsRequest{
				Filter: &v1.ListAgentsRequest_Filter{
					ModelIds: []string{"d3feed80-bb09-41b1-8cc7-b39022941565"},
				},
			},
		})
		if err != nil {
			slog.Error("failed to list agents", "error", err)
			return
		}

		agent := agentResp.Msg.Agents[0]

		resp, err := apiClient.Task().CreateTask(cmd.Context(), &connect.Request[v1.CreateTaskRequest]{
			Msg: &v1.CreateTaskRequest{
				AgentId: agent.Id,
			},
		})

		if err != nil {
			slog.Error("failed to create task", "error", err)
			return
		}

		p := tea.NewProgram(terminal.NewModel(cmd.Context(), apiClient, resp.Msg.Task, agent), tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running program: %v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
}
