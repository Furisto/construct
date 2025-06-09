package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type taskListOptions struct {
	Agent         string
	FormatOptions FormatOptions
}

func NewTaskListCmd() *cobra.Command {
	var options taskListOptions

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List tasks",
		Aliases: []string{"ls"},
		Example: `  # List all tasks
  construct task list

  # List tasks by agent name
  construct task list --agent "coder"

  # List tasks with YAML output
  construct task list --output yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			filter := &v1.ListTasksRequest_Filter{}

			if options.Agent != "" {
				agentID := options.Agent
				_, err := uuid.Parse(agentID)
				if err != nil {
					resolvedID, err := getAgentID(cmd.Context(), client, agentID)
					if err != nil {
						return fmt.Errorf("failed to resolve agent %s: %w", agentID, err)
					}
					agentID = resolvedID
				}
				filter.AgentId = &agentID
			}

			req := &connect.Request[v1.ListTasksRequest]{
				Msg: &v1.ListTasksRequest{
					Filter: filter,
				},
			}

			resp, err := client.Task().ListTasks(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to list tasks: %w", err)
			}

			displayTasks := make([]*DisplayTask, len(resp.Msg.Tasks))
			for i, task := range resp.Msg.Tasks {
				displayTasks[i] = ConvertTaskToDisplay(task)
			}

			return getFormatter(cmd.Context()).Display(displayTasks, options.FormatOptions.Output)
		},
	}

	cmd.Flags().StringVarP(&options.Agent, "agent", "a", "", "Filter by agent (name or ID)")
	addFormatOptions(cmd, &options.FormatOptions)
	return cmd
}
