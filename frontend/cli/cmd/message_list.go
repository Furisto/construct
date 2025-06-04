package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var messageListOptions struct {
	TaskId  string
	AgentId string
	Role    string
}

var messageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List messages",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		filter := &v1.ListMessagesRequest_Filter{}
		if messageListOptions.TaskId != "" {
			filter.TaskId = &messageListOptions.TaskId
		}
		if messageListOptions.AgentId != "" {
			filter.AgentId = &messageListOptions.AgentId
		}
		if messageListOptions.Role != "" {
			var role v1.MessageRole
			switch messageListOptions.Role {
			case "user":
				role = v1.MessageRole_MESSAGE_ROLE_USER
			case "assistant":
				role = v1.MessageRole_MESSAGE_ROLE_ASSISTANT
			default:
				role = v1.MessageRole_MESSAGE_ROLE_UNSPECIFIED
			}
			filter.Role = &role
		}

		resp, err := client.Message().ListMessages(cmd.Context(), &connect.Request[v1.ListMessagesRequest]{
			Msg: &v1.ListMessagesRequest{
				Filter: filter,
			},
		})

		if err != nil {
			return err
		}

		messages := make([]*DisplayMessage, len(resp.Msg.Messages))
		for i, message := range resp.Msg.Messages {
			messages[i] = ConvertMessageToDisplay(message)
		}

		return DisplayResources(messages, formatOptions.Output)
	},
}

func init() {
	messageListCmd.Flags().StringVarP(&messageListOptions.TaskId, "task-id", "t", "", "Filter messages by task ID")
	messageListCmd.Flags().StringVarP(&messageListOptions.AgentId, "agent-id", "a", "", "Filter messages by agent ID")
	messageListCmd.Flags().StringVarP(&messageListOptions.Role, "role", "r", "", "Filter messages by role (user, assistant)")
	addFormatOptions(messageListCmd)
	messageCmd.AddCommand(messageListCmd)
}
