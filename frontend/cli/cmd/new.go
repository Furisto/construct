package cmd

import (
	"context"
	"fmt"
	"log/slog"

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
		Short: "Launch a new interactive session with an agent",
		Long: `Launch a new interactive session with an agent.

Starts a real-time, interactive conversation with an AI agent in your terminal. 
This is the primary command for collaborative tasks like coding, debugging, and 
code reviews.`,
		Example: `  # Start a chat with the default agent
  construct new

  # Start a chat with a specific agent named 'coder'
  construct new --agent coder

  # Start a chat with an agent sandboxed in a different directory
  construct new --workspace /path/to/project`,
		GroupID: "core",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			userInfo := getUserInfo(cmd.Context())

			if options.workspace == "" {
				workspace, err := userInfo.Cwd()
				if err != nil {
					return err
				}
				options.workspace = workspace
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := getAPIClient(cmd.Context())
			verbose := getGlobalOptions(cmd.Context()).LogLevel == LogLevelDebug

			return fail.HandleError(cmd, handleNewCommand(cmd.Context(), apiClient, options, verbose))
		},
	}

	cmd.Flags().StringVar(&options.agent, "agent", "", "Start the session with a specific agent. Defaults to the last used agent")
	cmd.Flags().StringVar(&options.workspace, "workspace", "", "Set the agent's working directory. Defaults to the current directory")

	return cmd
}

func handleNewCommand(ctx context.Context, apiClient *api.Client, options *newOptions, verbose bool) error {
	slog.Info("starting new command", "agent", options.agent, "workspace", options.workspace)
	agentID, err := getAgentID(ctx, apiClient, options.agent)
	if err != nil {
		slog.Error("failed to get agent ID", "error", err, "agent", options.agent)
		return err
	}

	agentResp, err := apiClient.Agent().GetAgent(ctx, &connect.Request[v1.GetAgentRequest]{
		Msg: &v1.GetAgentRequest{
			Id: agentID,
		},
	})
	if err != nil {
		slog.Error("failed to retrieve agent", "error", err, "agent", options.agent)
		return err
	}

	agent := agentResp.Msg.Agent
	resp, err := apiClient.Task().CreateTask(ctx, &connect.Request[v1.CreateTaskRequest]{
		Msg: &v1.CreateTaskRequest{
			AgentId:          agent.Metadata.Id,
			ProjectDirectory: options.workspace,
		},
	})

	if err != nil {
		slog.Error("failed to create task", "error", err, "agent", options.agent)
		return err
	}

	model := terminal.NewSession(ctx, apiClient, resp.Msg.Task, agent)
	if verbose {
		model.Verbose = true
	}

	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)

	go func() {
		taskID := resp.Msg.Task.Metadata.Id
		watch, err := apiClient.Event().Subscribe(ctx, &connect.Request[v1.EventSubscribeRequest]{
			Msg: &v1.EventSubscribeRequest{
				EventTypes: []string{"task.*", "message.*"},
				TaskId:     &taskID,
			},
		})
		if err != nil {
			program.Send(terminal.NewError(err))
			return
		}

		defer watch.Close()

		for watch.Receive() {
			msg := watch.Msg()
			if msg.Event == nil {
				continue
			}
			switch payload := msg.Event.Payload.(type) {
			case *v1.Event_Message:
				if payload.Message != nil && payload.Message.Message != nil {
					program.Send(payload.Message.Message)
				}
			case *v1.Event_Task:
				if payload.Task != nil {
					program.Send(payload.Task)
				}
			}
		}

		if err := watch.Err(); err != nil {
			program.Send(terminal.NewError(err))
		}
	}()

	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
	}

	return nil
}
