package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var taskGetOptions struct {
	Id string
}

var taskGetCmd = &cobra.Command{
	Use:  "get",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient()

		resp, err := client.Task().GetTask(cmd.Context(), &connect.Request[v1.GetTaskRequest]{
			Msg: &v1.GetTaskRequest{Id: args[0]},
		})

		if err != nil {
			return err
		}

		return DisplayResources([]*DisplayTask{ConvertTaskToDisplay(resp.Msg.Task)}, formatOptions.Output)
	},
}

func init() {
	taskCmd.AddCommand(taskGetCmd)
}
