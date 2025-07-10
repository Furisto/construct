package cmd

import (
	"fmt"
	"os"

	"connectrpc.com/connect"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/spf13/cobra"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/terminal"
)

type newOptions struct {
	agent     string
	workspace string
}

func NewNewCmd() *cobra.Command {
	options := &newOptions{}

	cmd := &cobra.Command{
		Use:   "new [flags]",
		Short: "Start a new interactive conversation",
		Long: `Start a new interactive conversation.

Examples:
  # Start a new conversation with the default agent
  construct new

  # Start with a specific agent
  construct new --agent coder

  # Sandbox another directory
  construct new --workspace /workspace/repo/hello/world`,
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := getAPIClient(cmd.Context())

			agentID, err := getAgentID(cmd.Context(), apiClient, options.agent)
			if err != nil {
				return err
			}

			agentResp, err := apiClient.Agent().GetAgent(cmd.Context(), &connect.Request[v1.GetAgentRequest]{
				Msg: &v1.GetAgentRequest{
					Id: agentID,
				},
			})
			if err != nil {
				return err
			}

			agent := agentResp.Msg.Agent
			resp, err := apiClient.Task().CreateTask(cmd.Context(), &connect.Request[v1.CreateTaskRequest]{
				Msg: &v1.CreateTaskRequest{
					AgentId: agent.Metadata.Id,
				},
			})

			if err != nil {
				return err
			}

			tempFile, err := os.CreateTemp("", "construct-new-*")
			if err != nil {
				return err
			}

			fmt.Println("Temp file created", tempFile.Name())
			tea.LogToFile(tempFile.Name(), "debug")

			program := tea.NewProgram(
				terminal.NewModel(cmd.Context(), apiClient, resp.Msg.Task, agent),
				tea.WithAltScreen(),
			)

			if _, err := program.Run(); err != nil {
				fmt.Printf("Error running program: %v\n", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&options.agent, "agent", "", "Use a specific agent (default: last used or configured default)")
	cmd.Flags().StringVar(&options.workspace, "workspace", "", "The sandbox in which the agent can operate. It cannot see outside of the sandbox. If not specified the current directory is used")

	return cmd
}
