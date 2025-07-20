package agent

import (
	"encoding/json"
	"fmt"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	case types.MessageSourceUser, types.MessageSourceSystem:
		return model.MessageSourceUser, nil
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

		case types.MessageBlockKindCodeInterpreterCall:
			var toolCall model.ToolCallBlock
			err := json.Unmarshal([]byte(block.Payload), &toolCall)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal code interpreter call block: %w", err)
			}
			contentBlocks = append(contentBlocks, &toolCall)

		case types.MessageBlockKindCodeInterpreterResult:
			var interpreterResult InterpreterToolResult
			err := json.Unmarshal([]byte(block.Payload), &interpreterResult)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal code interpreter result block: %w", err)
			}
			contentBlocks = append(contentBlocks, &model.ToolResultBlock{
				ID:        interpreterResult.ID,
				Name:      "code_interpreter",
				Result:    interpreterResult.Output,
				Succeeded: interpreterResult.Error == nil,
			})
		default:
			return nil, fmt.Errorf("unknown message block kind: %s", block.Kind)
		}
	}

	return contentBlocks, nil
}

func ConvertMemoryMessageToProto(m *memory.Message) (*v1.Message, error) {
	var role v1.MessageRole
	switch m.Source {
	case types.MessageSourceUser:
		role = v1.MessageRole_MESSAGE_ROLE_USER
	case types.MessageSourceAssistant:
		role = v1.MessageRole_MESSAGE_ROLE_ASSISTANT
	default:
		role = v1.MessageRole_MESSAGE_ROLE_UNSPECIFIED
	}

	var contentParts []*v1.MessagePart
	for _, block := range m.Content.Blocks {
		switch block.Kind {
		case types.MessageBlockKindText:
			contentParts = append(contentParts, &v1.MessagePart{
				Data: &v1.MessagePart_Text_{
					Text: &v1.MessagePart_Text{
						Content: block.Payload,
					},
				},
			})
		case types.MessageBlockKindCodeInterpreterResult:
			var interpreterResult InterpreterToolResult
			err := json.Unmarshal([]byte(block.Payload), &interpreterResult)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal code interpreter result: %w", err)
			}

			contentParts = append(contentParts, &v1.MessagePart{
				Data: &v1.MessagePart_ToolResult{
					ToolResult: &v1.ToolResult{
						ToolName: "code_interpreter",
						Result: &v1.ToolResult_CodeInterpreter{
							CodeInterpreter: &v1.ToolResult_CodeInterpreterResult{
								Output: interpreterResult.Output,
							},
						},
					},
				},
			})
		}
	}

	messageUsage := &v1.MessageUsage{}
	if m.Usage != nil {
		messageUsage = &v1.MessageUsage{
			InputTokens:      m.Usage.InputTokens,
			OutputTokens:     m.Usage.OutputTokens,
			CacheWriteTokens: m.Usage.CacheWriteTokens,
		}
	}

	return &v1.Message{
		Metadata: &v1.MessageMetadata{
			Id:        m.ID.String(),
			CreatedAt: timestamppb.New(m.CreateTime),
			UpdatedAt: timestamppb.New(m.UpdateTime),
			TaskId:    m.TaskID.String(),
			AgentId: func() *string {
				if m.AgentID != uuid.Nil {
					s := m.AgentID.String()
					return &s
				}
				return nil
			}(),
			ModelId: func() *string {
				if m.ModelID != uuid.Nil {
					s := m.ModelID.String()
					return &s
				}
				return nil
			}(),
			Role: role,
		},
		Spec: &v1.MessageSpec{
			Content: contentParts,
		},
		Status: &v1.MessageStatus{
			Usage: messageUsage,
		},
	}, nil
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
		// case *model.ToolResultBlock:
		// 	payload, err := json.Marshal(b)
		// 	if err != nil {
		// 		return nil, fmt.Errorf("failed to marshal tool result block: %w", err)
		// 	}
		// 	messageBlocks = append(messageBlocks, types.MessageBlock{
		// 		Kind:    types.MessageBlockKindNativeToolResult,
		// 		Payload: string(payload),
		// 	})
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

// func ConvertToolResultsToProto(results []ToolResult) ([]*v1.MessagePart, error) {
// 	var protoToolResults []*v1.MessagePart
// 	for _, result := range results {
// 		switch result := result.(type) {
// 		case *InterpreterToolResult:
// 			interpreterFuncResults, err := convertFunctionCallsToProto(result.FunctionCalls)
// 			if err != nil {
// 				return nil, fmt.Errorf("failed to convert tool result to proto: %w", err)
// 			}
// 			protoToolResults = append(protoToolResults, interpreterFuncResults...)
// 		}
// 	}
// 	return protoToolResults, nil
// }

// func convertFunctionCallsToProto(functionCalls []codeact.FunctionCall) ([]*v1.MessagePart, error) {
// 	protoFunctionCalls := make([]*v1.MessagePart, 0, len(functionCalls))
// 	for _, call := range functionCalls {
// 		switch call.ToolName {
// 		case tool.ToolNameSubmitReport:
// 			submitReport, ok := call.Output.(*tool.SubmitReportResult)
// 			if !ok {
// 				return nil, fmt.Errorf("%s has invalid tool result type: %T", call.ToolName, call.Output)
// 			}
// 			protoFunctionCalls = append(protoFunctionCalls, &v1.MessagePart{
// 				Data: &v1.MessagePart_SubmitReport_{
// 					SubmitReport: &v1.MessagePart_SubmitReport{
// 						Summary:      submitReport.Summary,
// 						Completed:    submitReport.Completed,
// 						Deliverables: submitReport.Deliverables,
// 						NextSteps:    submitReport.NextSteps,
// 					},
// 				},
// 			})
// 		default:
// 			protoFunctionCalls = append(protoFunctionCalls, &v1.MessagePart{
// 				Data: &v1.MessagePart_ToolResult_{
// 					ToolResult: &v1.MessagePart_ToolResult{
// 						ToolName: v1.ToolName_UNSPECIFIED,
// 						Result:   fmt.Sprintf("%v", call.Output),
// 					},
// 				},
// 			})
// 		}
// 	}

// 	return protoFunctionCalls, nil
// }
