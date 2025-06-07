package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var taskGetOptions struct {
	Id            string
	FormatOptions FormatOptions
}

var taskGetCmd = &cobra.Command{
	Use:  "get",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		resp, err := client.Task().GetTask(cmd.Context(), &connect.Request[v1.GetTaskRequest]{
			Msg: &v1.GetTaskRequest{Id: args[0]},
		})

		if err != nil {
			return err
		}

		return getFormatter(cmd.Context()).Display([]*DisplayTask{ConvertTaskToDisplay(resp.Msg.Task)}, taskGetOptions.FormatOptions.Output)
	},
}

func init() {
	addFormatOptions(taskGetCmd, &taskGetOptions.FormatOptions)
	taskCmd.AddCommand(taskGetCmd)
}
