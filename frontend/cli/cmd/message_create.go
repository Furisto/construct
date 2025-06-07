package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var messageCreateOptions struct {
	FormatOptions FormatOptions
}

var messageCreateCmd = &cobra.Command{
	Use:   "create <task-id> <content>",
	Short: "Create a new message",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		resp, err := client.Message().CreateMessage(cmd.Context(), &connect.Request[v1.CreateMessageRequest]{
			Msg: &v1.CreateMessageRequest{
				TaskId:  args[0],
				Content: args[1],
			},
		})

		if err != nil {
			return err
		}

		fmt.Println(resp.Msg.Message.Id)
		return nil
	},
}

func init() {
	addFormatOptions(messageCreateCmd, &messageCreateOptions.FormatOptions)
	messageCmd.AddCommand(messageCreateCmd)
}
