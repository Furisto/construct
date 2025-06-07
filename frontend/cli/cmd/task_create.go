package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var taskCreateOptions struct {
	Name  string
	Agent string

}

var taskCreateCmd = &cobra.Command{
	Use: "create",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		_, err := client.Task().CreateTask(cmd.Context(), &connect.Request[v1.CreateTaskRequest]{
			Msg: &v1.CreateTaskRequest{
			},
		})

		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskCreateCmd)

	taskCreateCmd.Flags().StringVarP(&taskCreateOptions.Name, "name", "n", "", "The name of the task")
	taskCreateCmd.Flags().StringVarP(&taskCreateOptions.Agent, "agent", "a", "", "The agent to use")
}
