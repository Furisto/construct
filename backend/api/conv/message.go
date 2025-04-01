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

	var role v1.MessageRole
	switch m.Role {
	case types.MessageRoleUser:
		role = v1.MessageRole_MESSAGE_ROLE_USER
	case types.MessageRoleAssistant:
		role = v1.MessageRole_MESSAGE_ROLE_ASSISTANT
	default:
		role = v1.MessageRole_MESSAGE_ROLE_UNSPECIFIED
	}

	var usage *v1.MessageUsage
	if m.Usage != nil {
		usage = &v1.MessageUsage{
			InputTokens:      m.Usage.InputTokens,
			OutputTokens:     m.Usage.OutputTokens,
			CacheWriteTokens: m.Usage.CacheWriteTokens,
			CacheReadTokens:  m.Usage.CacheReadTokens,
			Cost:             m.Usage.Cost,
		}
	}

	var content string
	if m.Content != nil && len(m.Content.Blocks) > 0 {
		for _, block := range m.Content.Blocks {
			if block.Type == types.MessageContentBlockTypeText {
				content = block.Text
				break
			}
		}
	}

	return &v1.Message{
		Id: m.ID.String(),
		Metadata: &v1.MessageMetadata{
			CreatedAt: timestamppb.New(m.CreateTime),
			UpdatedAt: timestamppb.New(m.UpdateTime),
			TaskId:    m.TaskID.String(),
			AgentId:   m.AgentID.String(),
			Role:      role,
			Usage:     usage,
		},
		Content: content,
	}, nil
}