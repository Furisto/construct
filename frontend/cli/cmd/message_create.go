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
		Use:   "create <task-id> <content> [flags]",
		Short: "Add a message to a task programmatically",
		Long: `Add a message to a task programmatically.

Appends a new message to a task's history. This is an advanced command, typically 
used for scripting or integrating external tools with Construct tasks.`,
		Example: `  # Add a user message to an existing task
  construct message create "01974c1d-0be8-70e1-88b4-ad9462fff25e" "Please check the file again."`,
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
