package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type messageListOptions struct {
	Task          string
	Agent         string
	Role          string
	RenderOptions RenderOptions
}

func NewMessageListCmd() *cobra.Command {
	var options messageListOptions

	cmd := &cobra.Command{
		Use:     "list [flags]",
		Short:   "List messages",
		Aliases: []string{"ls"},
		Long: `List messages.

Lists messages, typically filtered by a specific task. Useful for reviewing or 
exporting a conversation history.`,
		Example: `  # List all messages for a specific task
  construct message list --task "01974c1d-0be8-70e1-88b4-ad9462fff25e"

  # List only the assistant's responses in that task
  construct message list --task "01974c1d-0be8-70e1-88b4-ad9462fff25e" --role assistant`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			filter := &v1.ListMessagesRequest_Filter{}

			if options.Task != "" {
				filter.TaskIds = &options.Task
			}

			if options.Agent != "" {
				agentID, err := getAgentID(cmd.Context(), client, options.Agent)
				if err != nil {
					return fmt.Errorf("failed to resolve agent %s: %w", options.Agent, err)
				}
				filter.AgentIds = &agentID
			}

			if options.Role != "" {
				switch options.Role {
				case "user":
					role := v1.MessageRole_MESSAGE_ROLE_USER
					filter.Roles = &role
				case "assistant":
					role := v1.MessageRole_MESSAGE_ROLE_ASSISTANT
					filter.Roles = &role
				default:
					return fmt.Errorf("invalid role %s: must be 'user' or 'assistant'", options.Role)
				}
			}

			req := &connect.Request[v1.ListMessagesRequest]{
				Msg: &v1.ListMessagesRequest{
					Filter: filter,
				},
			}

			resp, err := client.Message().ListMessages(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to list messages: %w", err)
			}

			displayMessages := make([]*DisplayMessage, len(resp.Msg.Messages))
			for i, message := range resp.Msg.Messages {
				displayMessages[i] = ConvertMessageToDisplay(message)
			}

			return getRenderer(cmd.Context()).Render(displayMessages, &options.RenderOptions)
		},
	}

	cmd.Flags().StringVarP(&options.Task, "task", "t", "", "(Recommended) Filter messages by task ID")
	cmd.Flags().StringVarP(&options.Agent, "agent", "a", "", "Filter by the agent that participated in the conversation")
	cmd.Flags().StringVarP(&options.Role, "role", "r", "", "Filter messages by the role of the author")
	addRenderOptions(cmd, &options.RenderOptions)

	return cmd
}
