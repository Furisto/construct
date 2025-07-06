package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type messageCreateOptions struct {
	RenderOptions RenderOptions
}

func NewMessageCreateCmd() *cobra.Command {
	var options messageCreateOptions

	cmd := &cobra.Command{
		Use:   "create <task-id> <content>",
		Short: "Create a new message",
		Long:  `Create a new message for a task by specifying the task ID and message content.`,
		Example: `  # Create a message for a task
  construct message create "123e4567-e89b-12d3-a456-426614174000" "Please implement a hello world function""`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			resp, err := client.Message().CreateMessage(cmd.Context(), &connect.Request[v1.CreateMessageRequest]{
				Msg: &v1.CreateMessageRequest{
					TaskId: args[0],
					Content: []*v1.MessagePart{
						{
							Data: &v1.MessagePart_Text_{
								Text: &v1.MessagePart_Text{
									Content: args[1],
								},
							},
						},
					},
				},
			})

			if err != nil {
				return fmt.Errorf("failed to create message: %w", err)
			}

			cmd.Println(resp.Msg.Message.Metadata.Id)
			return nil
		},
	}

	addRenderOptions(cmd, &options.RenderOptions)
	return cmd
}
