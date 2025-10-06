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
			endpointContext := getEndpointContext(cmd.Context())
			if endpointContext.Address == "" {
				return fmt.Errorf("endpoint context not found")
			}
			apiClient := api.NewClient(endpointContext, api.WithConnectOptions())
			verbose := getGlobalOptions(cmd.Context()).LogLevel == LogLevelDebug

			return fail.HandleError(handleNewCommand(cmd.Context(), apiClient, options, verbose))
		},
	}

	cmd.Flags().StringVar(&options.agent, "agent", "", "Start the session with a specific agent. Defaults to the last used agent")
	cmd.Flags().StringVar(&options.workspace, "workspace", "", "Set the agent's working directory. Defaults to the current directory")

	return cmd
}

func handleNewCommand(ctx context.Context, apiClient *api.Client, options *newOptions, verbose bool) error {
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
			AgentId:          agent.Metadata.Id,
			ProjectDirectory: options.workspace,
		},
	})

	if err != nil {
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

	fmt.Println("Subscribed to task", resp.Msg.Task.Metadata.Id)
	go func() {
		watch, err := apiClient.Task().Subscribe(ctx, &connect.Request[v1.SubscribeRequest]{
			Msg: &v1.SubscribeRequest{
				TaskId: resp.Msg.Task.Metadata.Id,
			},
		})
		if err != nil {
			program.Send(terminal.NewError(err))
			return
		}

		defer watch.Close()

		for watch.Receive() {
			msg := watch.Msg()
			switch msg.Event.(type) {
			case *v1.SubscribeResponse_Message:
				program.Send(msg.GetMessage())
			case *v1.SubscribeResponse_TaskEvent:
				program.Send(msg.GetTaskEvent())
			}
		}

		if err := watch.Err(); err != nil {
			program.Send(terminal.NewError(err))
		}
	}()

	tempFile, err := os.CreateTemp("", "construct-new-*")
	if err != nil {
		return err
	}

	tea.LogToFile(tempFile.Name(), "debug")

	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
	}

	return nil
}

type ErrorInterceptor struct {
}

func (e *ErrorInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func (e *ErrorInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (e *ErrorInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
