package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type taskDeleteOptions struct {
	Force bool
}

func NewTaskDeleteCmd() *cobra.Command {
	options := &taskDeleteOptions{}
	cmd := &cobra.Command{
		Use:     "delete <task-id>...",
		Aliases: []string{"rm"},
		Short:   "Delete one or more tasks by their IDs",
		Args:    cobra.MinimumNArgs(1),
		Example: `  # Delete a single task
  construct task delete 01974c1d-0be8-70e1-88b4-ad9462fff25e

  # Delete multiple tasks
  construct task delete 01974c1d-0be8-70e1-88b4-ad9462fff25e 01974c1d-0be8-70e1-88b4-ad9462fff26f`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			if !options.Force && !confirmDeletion(cmd.InOrStdin(), cmd.OutOrStdout(), "task", args) {
				return nil
			}

			for _, taskID := range args {
				_, err := client.Task().DeleteTask(cmd.Context(), &connect.Request[v1.DeleteTaskRequest]{
					Msg: &v1.DeleteTaskRequest{Id: taskID},
				})
				if err != nil {
					return fmt.Errorf("failed to delete task %s: %w", taskID, err)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Force deletion without confirmation")
	return cmd
}
