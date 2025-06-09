package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type taskGetOptions struct {
	FormatOptions FormatOptions
}

func NewTaskGetCmd() *cobra.Command {
	var options taskGetOptions

	cmd := &cobra.Command{
		Use:   "get <task-id>",
		Short: "Get a task by ID",
		Args:  cobra.ExactArgs(1),
		Example: `  # Get task by ID
  construct task get 01974c1d-0be8-70e1-88b4-ad9462fff25e

  # Get task with JSON output
  construct task get 01974c1d-0be8-70e1-88b4-ad9462fff25e --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())
			taskID := args[0]

			req := &connect.Request[v1.GetTaskRequest]{
				Msg: &v1.GetTaskRequest{Id: taskID},
			}

			resp, err := client.Task().GetTask(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to get task %s: %w", taskID, err)
			}

			displayTask := ConvertTaskToDisplay(resp.Msg.Task)
			return getFormatter(cmd.Context()).Display(displayTask, options.FormatOptions.Output)
		},
	}

	addFormatOptions(cmd, &options.FormatOptions)
	return cmd
}
