package cmd

import (
	"fmt"
	"path/filepath"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

type taskCreateOptions struct {
	Agent     string
	Workspace string
}

func NewTaskCreateCmd() *cobra.Command {
	var options taskCreateOptions

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new task",
		Example: `  # Create task and assign to agent by name
  construct task create --agent coder

  # Create task with both agent and workspace directory
  construct task create --agent sql-expert --workspace /path/to/repo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())
			fs := getFileSystem(cmd.Context())

			if options.Workspace != "" {
				exists, err := afero.Exists(fs, options.Workspace)
				if err != nil {
					return fmt.Errorf("failed to check if workspace directory %s exists: %w", options.Workspace, err)
				}
				if !exists {
					return fmt.Errorf("workspace directory %s does not exist", options.Workspace)
				}

				absPath, err := filepath.Abs(options.Workspace)
				if err != nil {
					return fmt.Errorf("failed to get absolute path of workspace directory %s: %w", options.Workspace, err)
				}

				options.Workspace = absPath
			}

			agentID := options.Agent
			if agentID != "" {
				_, err := uuid.Parse(agentID)
				if err != nil {
					resolvedID, err := getAgentID(cmd.Context(), client, agentID)
					if err != nil {
						return fmt.Errorf("failed to resolve agent %s: %w", agentID, err)
					}
					agentID = resolvedID
				}
			}

			req := &connect.Request[v1.CreateTaskRequest]{
				Msg: &v1.CreateTaskRequest{
					AgentId:          agentID,
					ProjectDirectory: options.Workspace,
				},
			}

			resp, err := client.Task().CreateTask(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to create task: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), resp.Msg.Task.Id)
			return nil
		},
	}

	cmd.Flags().StringVarP(&options.Agent, "agent", "a", "", "The agent to assign to the task (name or ID)")
	cmd.Flags().StringVarP(&options.Workspace, "workspace", "w", "", "The workspace directory")

	cmd.MarkFlagRequired("agent")

	return cmd
}
