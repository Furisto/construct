package conv

import (
	"encoding/json"
	"fmt"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
)

func ConvertMemoryMessageToModel(m *memory.Message) (*model.Message, error) {
	source := ConvertMemoryMessageSourceToModel(m.Source)
	contentBlocks, err := ConvertMemoryMessageBlocksToModel(m.Content.Blocks)
	if err != nil {
		return nil, fmt.Errorf("failed to convert memory message blocks to model: %w", err)
	}

	return &model.Message{
		Source:  source,
		Content: contentBlocks,
	}, nil
}

func ConvertMemoryMessageSourceToModel(source types.MessageSource) model.MessageSource {
	switch source {
	case types.MessageSourceAssistant:
		return model.MessageSourceModel
	case types.MessageSourceUser:
		return model.MessageSourceUser
	default:
		return model.MessageSourceUser
	}
}

func ConvertMemoryMessageBlocksToModel(blocks []types.MessageBlock) ([]model.ContentBlock, error) {
	contentBlocks := make([]model.ContentBlock, len(blocks))
	for _, block := range blocks {
		switch block.Kind {
		case types.MessageBlockKindText:
			contentBlocks = append(contentBlocks, &model.TextBlock{
				Text: block.Payload,
			})
		case types.MessageBlockKindNativeToolCall:
			var toolCall model.ToolCallBlock
			err := json.Unmarshal([]byte(block.Payload), &toolCall)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal native tool call block: %w", err)
			}
			contentBlocks = append(contentBlocks, &toolCall)
		case types.MessageBlockKindNativeToolResult:
			var toolResult model.ToolResultBlock
			err := json.Unmarshal([]byte(block.Payload), &toolResult)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal native tool result block: %w", err)
			}
			contentBlocks = append(contentBlocks, &toolResult)

		case types.MessageBlockKindCodeActToolCall,
			types.MessageBlockKindCodeActToolResult,
			types.MessageBlockKindCodeInterpreterCall,
			types.MessageBlockKindCodeInterpreterResult:
			continue
		default:
			return nil, fmt.Errorf("unknown message block kind: %s", block.Kind)
		}
	}

	return contentBlocks, nil
}

func ConvertModelMessageToMemory(m *model.Message) (*memory.Message, error) {
	source := ConvertModelMessageSourceToMemory(m.Source)
	blocks, err := ConvertModelContentBlocksToMemory(m.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to convert model content blocks to memory: %w", err)
	}

	return &memory.Message{
		Source:  source,
		Content: &types.MessageContent{
			Blocks: blocks,
		},
	}, nil
}

func ConvertModelMessageSourceToMemory(source model.MessageSource) types.MessageSource {
	switch source {
	case model.MessageSourceModel:
		return types.MessageSourceAssistant
	case model.MessageSourceUser:
		return types.MessageSourceUser
	default:
		return types.MessageSourceUser
	}
}

func ConvertModelContentBlocksToMemory(blocks []model.ContentBlock) ([]types.MessageBlock, error) {
	var messageBlocks []types.MessageBlock

	for _, block := range blocks {
		switch b := block.(type) {
		case *model.TextBlock:
			messageBlocks = append(messageBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindText,
				Payload: b.Text,
			})
		case *model.ToolCallBlock:
			payload, err := json.Marshal(b)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool call block: %w", err)
			}
			messageBlocks = append(messageBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindNativeToolCall,
				Payload: string(payload),
			})
		case *model.ToolResultBlock:
			payload, err := json.Marshal(b)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool result block: %w", err)
			}
			messageBlocks = append(messageBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindNativeToolResult,
				Payload: string(payload),
			})
		default:
			return nil, fmt.Errorf("unknown content block type: %T", block)
		}
	}

	return messageBlocks, nil
}
