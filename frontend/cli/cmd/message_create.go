package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var messageCreateOptions struct {
	TaskId  string
	Content string
}

var messageCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new message",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient()

		resp, err := client.Message().CreateMessage(cmd.Context(), &connect.Request[v1.CreateMessageRequest]{
			Msg: &v1.CreateMessageRequest{
				TaskId:  messageCreateOptions.TaskId,
				Content: messageCreateOptions.Content,
			},
		})

		if err != nil {
			return err
		}

		return DisplayResources([]*DisplayMessage{ConvertMessageToDisplay(resp.Msg.Message)}, formatOptions.Output)
	},
}

func init() {
	messageCreateCmd.Flags().StringVarP(&messageCreateOptions.TaskId, "task-id", "t", "", "The ID of the task to create the message for")
	messageCreateCmd.Flags().StringVarP(&messageCreateOptions.Content, "content", "c", "", "The content of the message")
	messageCreateCmd.MarkFlagRequired("task-id")
	messageCreateCmd.MarkFlagRequired("content")
	addFormatOptions(messageCreateCmd)
	messageCmd.AddCommand(messageCreateCmd)
}
