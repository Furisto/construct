package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var messageGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a message",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		resp, err := client.Message().GetMessage(cmd.Context(), &connect.Request[v1.GetMessageRequest]{
			Msg: &v1.GetMessageRequest{Id: args[0]},
		})

		if err != nil {
			return err
		}

		return DisplayResources([]*DisplayMessage{ConvertMessageToDisplay(resp.Msg.Message)}, formatOptions.Output)
	},
}

func init() {
	addFormatOptions(messageGetCmd)
	messageCmd.AddCommand(messageGetCmd)
}
