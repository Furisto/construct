package cmd

import (
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var messageCmd = &cobra.Command{
	Use:   "message",
	Short: "Manage messages",
}

func init() {
	rootCmd.AddCommand(messageCmd)
}

type DisplayMessage struct {
	Id        string              `json:"id" yaml:"id"`
	TaskId    string              `json:"task_id" yaml:"task_id"`
	AgentId   string              `json:"agent_id" yaml:"agent_id"`
	ModelId   string              `json:"model_id" yaml:"model_id"`
	Role      string              `json:"role" yaml:"role"`
	Content   string              `json:"content" yaml:"content"`
	CreatedAt time.Time           `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time           `json:"updated_at" yaml:"updated_at"`
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
	return &DisplayMessage{
		Id:        message.Id,
		TaskId:    message.Metadata.TaskId,
		AgentId:   PtrToString(message.Metadata.AgentId),
		ModelId:   PtrToString(message.Metadata.ModelId),
		Role:      ConvertMessageRoleToString(message.Metadata.Role),
		Content:   message.Content.GetText(),
		CreatedAt: message.Metadata.CreatedAt.AsTime(),
		UpdatedAt: message.Metadata.UpdatedAt.AsTime(),
		Usage:     ConvertMessageUsageToDisplay(message.Metadata.Usage),
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
