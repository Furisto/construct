package cmd

import (
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

func NewMessageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "message",
		Short:   "Manage messages",
		Aliases: []string{"messages", "msg"},
		GroupID: "resource",
	}

	cmd.AddCommand(NewMessageCreateCmd())
	cmd.AddCommand(NewMessageGetCmd())
	cmd.AddCommand(NewMessageListCmd())
	cmd.AddCommand(NewMessageDeleteCmd())

	return cmd
}

type DisplayMessage struct {
	ID        string              `json:"id" yaml:"id" detail:"default"`
	TaskID    string              `json:"task_id" yaml:"task_id" detail:"default"`
	Agent     string              `json:"agent" yaml:"agent" detail:"default"`
	Model     string              `json:"model" yaml:"model" detail:"default"`
	Role      string              `json:"role" yaml:"role" detail:"default"`
	Content   string              `json:"content" yaml:"content"`
	CreatedAt time.Time           `json:"created_at" yaml:"created_at" detail:"full"`
	UpdatedAt time.Time           `json:"updated_at" yaml:"updated_at" detail:"full"`
	Usage     DisplayMessageUsage `json:"usage" yaml:"usage"`
}

type DisplayMessageUsage struct {
	InputTokens      int64   `json:"input_tokens" yaml:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens" yaml:"output_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens" yaml:"cache_write_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens" yaml:"cache_read_tokens"`
	Cost             float64 `json:"cost" yaml:"cost"`
}

func ConvertMessageToDisplay(message *v1.Message) *DisplayMessage {
	var agentId, modelId *string
	if message.Metadata.AgentId != nil {
		agentId = message.Metadata.AgentId
	}
	if message.Metadata.ModelId != nil {
		modelId = message.Metadata.ModelId
	}

	var usage DisplayMessageUsage
	if message.Status != nil && message.Status.Usage != nil {
		usage = ConvertMessageUsageToDisplay(message.Status.Usage)
	}

	content := ""
	for _, part := range message.Spec.Content {
		content += part.GetText().GetContent()
	}

	return &DisplayMessage{
		ID:        message.Metadata.Id,
		TaskID:    message.Metadata.TaskId,
		Agent:     PtrToString(agentId),
		Model:     PtrToString(modelId),
		Role:      ConvertMessageRoleToString(message.Metadata.Role),
		Content:   content,
		CreatedAt: message.Metadata.CreatedAt.AsTime(),
		UpdatedAt: message.Metadata.UpdatedAt.AsTime(),
		Usage:     usage,
	}
}

func ConvertMessageUsageToDisplay(usage *v1.MessageUsage) DisplayMessageUsage {
	if usage == nil {
		return DisplayMessageUsage{}
	}

	return DisplayMessageUsage{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		Cost:             usage.Cost,
	}
}

func ConvertMessageRoleToString(role v1.MessageRole) string {
	switch role {
	case v1.MessageRole_MESSAGE_ROLE_USER:
		return "user"
	case v1.MessageRole_MESSAGE_ROLE_ASSISTANT:
		return "assistant"
	default:
		return "unknown"
	}
}
