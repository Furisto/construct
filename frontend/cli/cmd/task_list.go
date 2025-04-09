package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)


var taskListCmd = &cobra.Command{
	Use: "list",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		resp, err := client.Task().ListTasks(cmd.Context(), &connect.Request[v1.ListTasksRequest]{})
		if err != nil {
			return err
		}

		tasks := make([]*DisplayTask, len(resp.Msg.Tasks))
		for i, task := range resp.Msg.Tasks {
			tasks[i] = ConvertTaskToDisplay(task)
		}

		return DisplayResources(tasks, formatOptions.Output)
	},
}

func init() {
	taskCmd.AddCommand(taskListCmd)
}
