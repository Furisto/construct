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
		Use:     "list",
		Short:   "List messages",
		Aliases: []string{"ls"},
		Long:    `List messages with optional filtering by task, agent, and role.`,
		Example: `  # List all messages
  construct message list

  # List messages by agent name
  construct message list --agent "coder"

  # List messages by task ID
  construct message list --task "456e7890-e12b-34c5-a678-901234567890"

  # List only assistant messages
  construct message list --role assistant`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			filter := &v1.ListMessagesRequest_Filter{}

			if options.Task != "" {
				filter.TaskId = &options.Task
			}

			if options.Agent != "" {
				agentID, err := getAgentID(cmd.Context(), client, options.Agent)
				if err != nil {
					return fmt.Errorf("failed to resolve agent %s: %w", options.Agent, err)
				}
				filter.AgentId = &agentID
			}

			if options.Role != "" {
				switch options.Role {
				case "user":
					role := v1.MessageRole_MESSAGE_ROLE_USER
					filter.Role = &role
				case "assistant":
					role := v1.MessageRole_MESSAGE_ROLE_ASSISTANT
					filter.Role = &role
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

	cmd.Flags().StringVarP(&options.Task, "task", "t", "", "Filter by task ID")
	cmd.Flags().StringVarP(&options.Agent, "agent", "a", "", "Filter by agent name or ID")
	cmd.Flags().StringVarP(&options.Role, "role", "r", "", "Filter by role (user or assistant)")
	addRenderOptions(cmd, &options.RenderOptions)

	return cmd
}
