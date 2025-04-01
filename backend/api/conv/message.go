package conv

import (
	"fmt"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ConvertMessageToProto(m *memory.Message) (*v1.Message, error) {
	if m == nil {
		return nil, fmt.Errorf("message is nil")
	}

	return &v1.Message{
		Id: m.ID.String(),
		Metadata: &v1.MessageMetadata{
			CreatedAt: timestamppb.New(m.CreateTime),
			UpdatedAt: timestamppb.New(m.UpdateTime),
			TaskId:    m.TaskID.String(),
			AgentId:   ConvertUUIDPtrToStringPtr(m.AgentID),
			ModelId:   ConvertUUIDPtrToStringPtr(m.ModelID),
			Role:      convertRole(m.Role),
			Usage:     convertUsage(m.Usage),
		},
		Content: &v1.MessageContent{
			Content: &v1.MessageContent_Text{
				Text: convertContent(m.Content),
			},
		},
	}, nil
}

func convertRole(role types.MessageRole) v1.MessageRole {
	switch role {
	case types.MessageRoleUser:
		return v1.MessageRole_MESSAGE_ROLE_USER
	case types.MessageRoleAssistant:
		return v1.MessageRole_MESSAGE_ROLE_ASSISTANT
	default:
		return v1.MessageRole_MESSAGE_ROLE_UNSPECIFIED
	}
}

func convertUsage(usage *types.MessageUsage) *v1.MessageUsage {
	if usage == nil {
		return nil
	}

	return &v1.MessageUsage{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		Cost:             usage.Cost,
	}
}

func convertContent(content *types.MessageContent) string {
	if content == nil || len(content.Blocks) == 0 {
		return ""
	}

	for _, block := range content.Blocks {
		if block.Type == types.MessageContentBlockTypeText {
			return block.Text
		}
	}
	return ""
}
