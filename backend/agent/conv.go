package agent

import (
	"encoding/json"
	"fmt"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
	tooltypes "github.com/furisto/construct/backend/tool/types"
)

func ConvertMemoryMessageToModel(m *memory.Message) (*model.Message, error) {
	source, err := ConvertMemoryMessageSourceToModel(m.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to convert memory message source to model: %w", err)
	}
	contentBlocks, err := ConvertMemoryMessageBlocksToModel(m.Content.Blocks)
	if err != nil {
		return nil, fmt.Errorf("failed to convert memory message blocks to model: %w", err)
	}

	return &model.Message{
		Source:  source,
		Content: contentBlocks,
	}, nil
}

func ConvertMemoryMessageSourceToModel(source types.MessageSource) (model.MessageSource, error) {
	switch source {
	case types.MessageSourceAssistant:
		return model.MessageSourceModel, nil
	case types.MessageSourceUser:
		return model.MessageSourceUser, nil
	case types.MessageSourceSystem:
		return model.MessageSourceSystem, nil
	default:
		return "", fmt.Errorf("unknown message source: %s", source)
	}
}

func ConvertMemoryMessageBlocksToModel(blocks []types.MessageBlock) ([]model.ContentBlock, error) {
	var contentBlocks []model.ContentBlock
	for _, block := range blocks {
		switch block.Kind {
		case types.MessageBlockKindText:
			contentBlocks = append(contentBlocks, &model.TextBlock{
				Text: block.Payload,
			})
		case types.MessageBlockKindToolCall:
			var toolCall tooltypes.ToolCall
			err := json.Unmarshal([]byte(block.Payload), &toolCall)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal tool call block: %w", err)
			}

			// Serialize Input back to raw JSON for model layer
			var args json.RawMessage
			if toolCall.Input != nil && toolCall.Input.Interpreter != nil {
				args, _ = json.Marshal(toolCall.Input.Interpreter)
			}

			contentBlocks = append(contentBlocks, &model.ToolCallBlock{
				ID:   toolCall.Provider.ID,
				Tool: toolCall.Tool,
				Args: args,
			})

		case types.MessageBlockKindToolResult:
			var toolResult tooltypes.ToolResult
			err := json.Unmarshal([]byte(block.Payload), &toolResult)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal tool result block: %w", err)
			}

			// Serialize Output back to string for model layer
			var resultStr string
			if toolResult.Output != nil && toolResult.Output.Interpreter != nil {
				resultStr = toolResult.Output.Interpreter.ConsoleOutput
				// Append function call errors if any
				for _, fc := range toolResult.Output.Interpreter.FunctionCalls {
					if fc.Output.ExecuteCommand != nil && fc.Output.ExecuteCommand.Stderr != "" {
						resultStr += "\n" + fc.Output.ExecuteCommand.Stderr
					}
				}
			}

			contentBlocks = append(contentBlocks, &model.ToolResultBlock{
				ID:        toolResult.Provider.ID,
				Name:      toolResult.Tool,
				Result:    resultStr,
				Succeeded: toolResult.Succeeded,
			})
		default:
			return nil, fmt.Errorf("unknown message block kind: %s", block.Kind)
		}
	}

	return contentBlocks, nil
}

// ConvertMemoryBlocksToAgent converts memory blocks to agent-typed blocks with parsed Input/Output.
func ConvertMemoryBlocksToAgent(blocks []types.MessageBlock) ([]*tooltypes.ToolCall, []*tooltypes.ToolResult, error) {
	var toolCalls []*tooltypes.ToolCall
	var toolResults []*tooltypes.ToolResult

	for _, block := range blocks {
		switch block.Kind {
		case types.MessageBlockKindToolCall:
			var toolCall tooltypes.ToolCall
			if err := json.Unmarshal([]byte(block.Payload), &toolCall); err != nil {
				return nil, nil, fmt.Errorf("failed to unmarshal tool call block: %w", err)
			}
			toolCalls = append(toolCalls, &toolCall)

		case types.MessageBlockKindToolResult:
			var toolResult tooltypes.ToolResult
			if err := json.Unmarshal([]byte(block.Payload), &toolResult); err != nil {
				return nil, nil, fmt.Errorf("failed to unmarshal tool result block: %w", err)
			}
			toolResults = append(toolResults, &toolResult)
		}
	}

	return toolCalls, toolResults, nil
}

func ConvertModelMessageToMemory(m *model.Message) (*memory.Message, error) {
	source := ConvertModelMessageSourceToMemory(m.Source)
	content, err := ConvertModelContentBlocksToMemory(m.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to convert model content blocks to memory: %w", err)
	}

	return &memory.Message{
		Source:  source,
		Content: content,
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

func ConvertModelContentBlocksToMemory(blocks []model.ContentBlock) (*types.MessageContent, error) {
	return ConvertModelContentBlocksToMemoryWithProvider(blocks, "")
}

func ConvertModelContentBlocksToMemoryWithProvider(blocks []model.ContentBlock, providerKind string) (*types.MessageContent, error) {
	var messageBlocks []types.MessageBlock

	for _, block := range blocks {
		switch b := block.(type) {
		case *model.TextBlock:
			messageBlocks = append(messageBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindText,
				Payload: b.Text,
			})
		case *model.ToolCallBlock:
			// Convert to unified ToolCall format
			toolCall := &tooltypes.ToolCall{
				Tool: b.Tool,
				Provider: &tooltypes.ProviderData{
					Kind: providerKind,
					ID:   b.ID,
				},
			}
			// Parse Args based on tool type
			if b.Tool == "code_interpreter" {
				var input tooltypes.InterpreterInput
				if err := json.Unmarshal(b.Args, &input); err == nil {
					toolCall.Input = &tooltypes.ToolInput{Interpreter: &input}
				}
			}
			payload, err := json.Marshal(toolCall)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool call block: %w", err)
			}
			messageBlocks = append(messageBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindToolCall,
				Payload: string(payload),
			})
		case *model.ToolResultBlock:
			// Convert to unified ToolResult format
			toolResult := &tooltypes.ToolResult{
				Tool:      b.Name,
				Succeeded: b.Succeeded,
				Provider: &tooltypes.ProviderData{
					Kind: providerKind,
					ID:   b.ID,
				},
			}
			payload, err := json.Marshal(toolResult)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool result block: %w", err)
			}
			messageBlocks = append(messageBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindToolResult,
				Payload: string(payload),
			})
		default:
			return nil, fmt.Errorf("unknown content block type: %T", block)
		}
	}

	return &types.MessageContent{
		Blocks: messageBlocks,
	}, nil
}

func ConvertModelUsageToMemory(usage *model.Usage) *types.MessageUsage {
	return &types.MessageUsage{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
	}
}

func convertTaskPhaseToMemory(phase TaskPhase) types.TaskPhase {
	switch phase {
	case TaskPhaseAwaitInput:
		return types.TaskPhaseAwaiting
	case TaskPhaseExecuteTools, TaskPhaseInvokeModel:
		return types.TaskPhaseRunning
	case TaskPhaseSuspended:
		return types.TaskPhaseSuspended
	}

	return types.TaskPhaseUnspecified
}
