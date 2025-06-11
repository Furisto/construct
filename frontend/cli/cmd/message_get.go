package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type messageGetOptions struct {
	RenderOptions RenderOptions
}

func NewMessageGetCmd() *cobra.Command {
	var options messageGetOptions

	cmd := &cobra.Command{
		Use:   "get <message-id>",
		Short: "Get a message by ID",
		Long:  `Get detailed information about a message by specifying its ID.`,
		Example: `  # Get a message by ID
  construct message get "123e4567-e89b-12d3-a456-426614174000"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			resp, err := client.Message().GetMessage(cmd.Context(), &connect.Request[v1.GetMessageRequest]{
				Msg: &v1.GetMessageRequest{Id: args[0]},
			})

			if err != nil {
				return fmt.Errorf("failed to get message %s: %w", args[0], err)
			}

			displayMessage := ConvertMessageToDisplay(resp.Msg.Message)
			return getRenderer(cmd.Context()).Render(displayMessage, &options.RenderOptions)
		},
	}

	addRenderOptions(cmd, WithCardFormat(&options.RenderOptions))
	return cmd
}
