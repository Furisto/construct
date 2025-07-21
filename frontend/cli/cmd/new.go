package cmd

import (
	"context"
	"fmt"
	"os"

	"connectrpc.com/connect"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/spf13/cobra"

	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
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

			return fail.HandleError(handleNewCommand(cmd.Context(), apiClient, options))
		},
	}

	cmd.Flags().StringVar(&options.agent, "agent", "", "Use a specific agent (default: last used or configured default)")
	cmd.Flags().StringVar(&options.workspace, "workspace", "", "The sandbox in which the agent can operate. It cannot see outside of the sandbox. If not specified the current directory is used")

	return cmd
}

func handleNewCommand(ctx context.Context, apiClient *api.Client, options *newOptions) error {
	agentID, err := getAgentID(ctx, apiClient, options.agent)
	if err != nil {
		return err
	}

	agentResp, err := apiClient.Agent().GetAgent(ctx, &connect.Request[v1.GetAgentRequest]{
		Msg: &v1.GetAgentRequest{
			Id: agentID,
		},
	})
	if err != nil {
		return err
	}

	agent := agentResp.Msg.Agent
	resp, err := apiClient.Task().CreateTask(ctx, &connect.Request[v1.CreateTaskRequest]{
		Msg: &v1.CreateTaskRequest{
			AgentId:     agent.Metadata.Id,
			Description: "Build a Go-based coding agent with Anthropic and OpenAI API integration",
		},
	})

	if err != nil {
		return err
	}

	fmt.Println("Created task", resp.Msg.Task.Metadata.Id)

	program := tea.NewProgram(
		terminal.NewModel(ctx, apiClient, resp.Msg.Task, agent),
		tea.WithAltScreen(),
	)

	fmt.Println("Subscribed to task", resp.Msg.Task.Metadata.Id)
	go func() {
		watch, err := apiClient.Task().Subscribe(ctx, &connect.Request[v1.SubscribeRequest]{
			Msg: &v1.SubscribeRequest{
				TaskId: resp.Msg.Task.Metadata.Id,
			},
		})
		if err != nil {
			fmt.Println("error subscribing to task:", err)
			return
		}

		defer watch.Close()

		for watch.Receive() {
			msg := watch.Msg()
			program.Send(msg.Message)
		}

		if err := watch.Err(); err != nil {
			fmt.Println("error watching task:", err)
		}
	}()
	fmt.Println("Running program", resp.Msg.Task.Metadata.Id)

	tempFile, err := os.CreateTemp("", "construct-new-*")
	if err != nil {
		return err
	}

	fmt.Println("Temp file created", tempFile.Name())

	tea.LogToFile(tempFile.Name(), "debug")

	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
	}

	return nil
}
