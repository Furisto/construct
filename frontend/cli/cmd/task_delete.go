package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var taskDeleteOptions struct {
	Id string
}

var taskDeleteCmd = &cobra.Command{
	Use: "delete",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		_, err := client.Task().DeleteTask(cmd.Context(), &connect.Request[v1.DeleteTaskRequest]{
			Msg: &v1.DeleteTaskRequest{Id: args[0]},
		})

		return err
	},
}

func init() {
	taskCmd.AddCommand(taskDeleteCmd)
}
