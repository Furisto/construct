package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var taskListOptions struct {
	FormatOptions FormatOptions
}

var taskListCmd = &cobra.Command{
	Use: "list",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		resp, err := client.Task().ListTasks(cmd.Context(), &connect.Request[v1.ListTasksRequest]{})
		if err != nil {
			return err
		}

		tasks := make([]*DisplayTask, len(resp.Msg.Tasks))
		for i, task := range resp.Msg.Tasks {
			tasks[i] = ConvertTaskToDisplay(task)
		}

		return getFormatter(cmd.Context()).Display(tasks, taskListOptions.FormatOptions.Output)
	},
}

func init() {
	addFormatOptions(taskListCmd, &taskListOptions.FormatOptions)
	taskCmd.AddCommand(taskListCmd)
}
