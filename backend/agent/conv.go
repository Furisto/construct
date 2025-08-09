package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
	toolbase "github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/codeact"
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
			var interpreterResult codeact.InterpreterToolResult
			err := json.Unmarshal([]byte(block.Payload), &interpreterResult)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal code interpreter result block: %w", err)
			}
			contentBlocks = append(contentBlocks, &model.ToolResultBlock{
				ID:        interpreterResult.ID,
				Name:      "code_interpreter",
				Result:    interpreterResult.Output,
				Succeeded: interpreterResult.Error == "",
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
	case types.MessageSourceSystem:
		role = v1.MessageRole_MESSAGE_ROLE_SYSTEM
	default:
		return nil, fmt.Errorf("unknown message source: %s", m.Source)
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

		case types.MessageBlockKindCodeInterpreterCall:
			var toolCall model.ToolCallBlock
			err := json.Unmarshal([]byte(block.Payload), &toolCall)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal code interpreter call block: %w", err)
			}

			var interpreterArgs codeact.InterpreterArgs
			err = json.Unmarshal(toolCall.Args, &interpreterArgs)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal code interpreter args: %w", err)
			}

			contentParts = append(contentParts, &v1.MessagePart{
				Data: &v1.MessagePart_ToolCall{
					ToolCall: &v1.ToolCall{
						ToolName: toolCall.Tool,
						Input: &v1.ToolCall_CodeInterpreter{
							CodeInterpreter: &v1.ToolCall_CodeInterpreterInput{
								Code: interpreterArgs.Script,
							},
						},
					},
				},
			})
		case types.MessageBlockKindCodeInterpreterResult:
			var interpreterResult codeact.InterpreterToolResult
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

			for _, call := range interpreterResult.FunctionCalls {
				switch call.ToolName {
				case toolbase.ToolNameCreateFile:
					createFileInput := call.Input.CreateFile
					if createFileInput == nil {
						slog.Error("create file input not set")
						continue
					}
					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_CreateFile{
									CreateFile: &v1.ToolCall_CreateFileInput{
										Path:    createFileInput.Path,
										Content: createFileInput.Content,
									},
								},
							},
						},
					})
					createFileResult := call.Output.CreateFile
					if createFileResult == nil {
						slog.Error("create file result not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolResult{
							ToolResult: &v1.ToolResult{
								ToolName: call.ToolName,
								Result: &v1.ToolResult_CreateFile{
									CreateFile: &v1.ToolResult_CreateFileResult{
										Overwritten: createFileResult.Overwritten,
									},
								},
							},
						},
					})
				case toolbase.ToolNameEditFile:
					editFileInput := call.Input.EditFile
					if editFileInput == nil {
						slog.Error("edit file input not set")
						continue
					}

					var diffs []*v1.ToolCall_EditFileInput_DiffPair
					for _, diff := range editFileInput.Diffs {
						diffs = append(diffs, &v1.ToolCall_EditFileInput_DiffPair{
							Old: diff.Old,
							New: diff.New,
						})
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_EditFile{
									EditFile: &v1.ToolCall_EditFileInput{
										Path:  editFileInput.Path,
										Diffs: diffs,
									},
								},
							},
						},
					})

					editFileResult := call.Output.EditFile
					if editFileResult == nil {
						slog.Error("edit file result not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolResult{
							ToolResult: &v1.ToolResult{
								ToolName: call.ToolName,
								Result: &v1.ToolResult_EditFile{
									EditFile: &v1.ToolResult_EditFileResult{
										Path: editFileResult.Path,
										PatchInfo: &v1.ToolResult_EditFileResult_PatchInfo{
											Patch:        editFileResult.PatchInfo.Patch,
											LinesAdded:   int32(editFileResult.PatchInfo.LinesAdded),
											LinesRemoved: int32(editFileResult.PatchInfo.LinesRemoved),
										},
									},
								},
							},
						},
					})
				case toolbase.ToolNameExecuteCommand:
					executeCommandInput := call.Input.ExecuteCommand
					if executeCommandInput == nil {
						slog.Error("execute command input not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_ExecuteCommand{
									ExecuteCommand: &v1.ToolCall_ExecuteCommandInput{
										Command: executeCommandInput.Command,
									},
								},
							},
						},
					})

					executeCommandResult := call.Output.ExecuteCommand
					if executeCommandResult == nil {
						slog.Error("execute command result not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolResult{
							ToolResult: &v1.ToolResult{
								ToolName: call.ToolName,
								Result: &v1.ToolResult_ExecuteCommand{
									ExecuteCommand: &v1.ToolResult_ExecuteCommandResult{
										Stdout:   executeCommandResult.Stdout,
										Stderr:   executeCommandResult.Stderr,
										ExitCode: int32(executeCommandResult.ExitCode),
										Command:  executeCommandResult.Command,
									},
								},
							},
						},
					})
				case toolbase.ToolNameFindFile:
					findFileInput := call.Input.FindFile
					if findFileInput == nil {
						slog.Error("find file input not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_FindFile{
									FindFile: &v1.ToolCall_FindFileInput{
										Pattern:        findFileInput.Pattern,
										Path:           findFileInput.Path,
										ExcludePattern: findFileInput.ExcludePattern,
										MaxResults:     int32(findFileInput.MaxResults),
									},
								},
							},
						},
					})

					findFileResult := call.Output.FindFile
					if findFileResult == nil {
						slog.Error("find file result not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolResult{
							ToolResult: &v1.ToolResult{
								ToolName: call.ToolName,
								Result: &v1.ToolResult_FindFile{
									FindFile: &v1.ToolResult_FindFileResult{
										Files:          findFileResult.Files,
										TotalFiles:     int32(findFileResult.TotalFiles),
										TruncatedCount: int32(findFileResult.TruncatedCount),
									},
								},
							},
						},
					})
				case toolbase.ToolNameGrep:
					grepInput := call.Input.Grep
					if grepInput == nil {
						slog.Error("grep input not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_Grep{
									Grep: &v1.ToolCall_GrepInput{
										Query:          grepInput.Query,
										Path:           grepInput.Path,
										IncludePattern: grepInput.IncludePattern,
										ExcludePattern: grepInput.ExcludePattern,
										CaseSensitive:  grepInput.CaseSensitive,
										MaxResults:     int32(grepInput.MaxResults),
									},
								},
							},
						},
					})

					grepResult := call.Output.Grep
					if grepResult == nil {
						slog.Error("grep result not set")
						continue
					}

					var matches []*v1.ToolResult_GrepResult_GrepMatch
					for _, match := range grepResult.Matches {
						var contextLines []*v1.ToolResult_GrepResult_ContextLine
						for _, context := range match.Context {
							contextLines = append(contextLines, &v1.ToolResult_GrepResult_ContextLine{
								LineNumber: int32(context.LineNumber),
								Content:    context.Content,
							})
						}
						matches = append(matches, &v1.ToolResult_GrepResult_GrepMatch{
							FilePath:    match.FilePath,
							LineNumber:  int32(match.LineNumber),
							LineContent: match.LineContent,
							Context:     contextLines,
						})
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolResult{
							ToolResult: &v1.ToolResult{
								ToolName: call.ToolName,
								Result: &v1.ToolResult_Grep{
									Grep: &v1.ToolResult_GrepResult{
										Matches:       matches,
										TotalMatches:  int32(grepResult.TotalMatches),
										SearchedFiles: int32(grepResult.SearchedFiles),
									},
								},
							},
						},
					})
				case toolbase.ToolNameListFiles:
					listFilesInput := call.Input.ListFiles
					if listFilesInput == nil {
						slog.Error("list files input not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_ListFiles{
									ListFiles: &v1.ToolCall_ListFilesInput{
										Path:      listFilesInput.Path,
										Recursive: listFilesInput.Recursive,
									},
								},
							},
						},
					})

					listFilesResult := call.Output.ListFiles
					if listFilesResult == nil {
						slog.Error("list files result not set")
						continue
					}

					var entries []*v1.ToolResult_ListFilesResult_DirectoryEntry
					for _, entry := range listFilesResult.Entries {
						entries = append(entries, &v1.ToolResult_ListFilesResult_DirectoryEntry{
							Name: entry.Name,
							Type: entry.Type,
							Size: entry.Size,
						})
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolResult{
							ToolResult: &v1.ToolResult{
								ToolName: call.ToolName,
								Result: &v1.ToolResult_ListFiles{
									ListFiles: &v1.ToolResult_ListFilesResult{
										Path:    listFilesResult.Path,
										Entries: entries,
									},
								},
							},
						},
					})
				case toolbase.ToolNameReadFile:
					readFileInput := call.Input.ReadFile
					if readFileInput == nil {
						slog.Error("read file input not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_ReadFile{
									ReadFile: &v1.ToolCall_ReadFileInput{
										Path: readFileInput.Path,
									},
								},
							},
						},
					})

					readFileResult := call.Output.ReadFile
					if readFileResult == nil {
						slog.Error("read file result not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolResult{
							ToolResult: &v1.ToolResult{
								ToolName: call.ToolName,
								Result: &v1.ToolResult_ReadFile{
									ReadFile: &v1.ToolResult_ReadFileResult{
										Path:    readFileResult.Path,
										Content: readFileResult.Content,
									},
								},
							},
						},
					})
				case toolbase.ToolNameSubmitReport:
					submitReportInput := call.Input.SubmitReport
					if submitReportInput == nil {
						slog.Error("submit report input not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_SubmitReport{
									SubmitReport: &v1.ToolCall_SubmitReportInput{
										Summary:      submitReportInput.Summary,
										Completed:    submitReportInput.Completed,
										Deliverables: submitReportInput.Deliverables,
										NextSteps:    submitReportInput.NextSteps,
									},
								},
							},
						},
					})

					submitReportResult := call.Output.SubmitReport
					if submitReportResult == nil {
						slog.Error("submit report result not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolResult{
							ToolResult: &v1.ToolResult{
								ToolName: call.ToolName,
								Result: &v1.ToolResult_SubmitReport{
									SubmitReport: &v1.ToolResult_SubmitReportResult{
										Summary:      submitReportResult.Summary,
										Completed:    submitReportResult.Completed,
										Deliverables: submitReportResult.Deliverables,
										NextSteps:    submitReportResult.NextSteps,
									},
								},
							},
						},
					})
				case toolbase.ToolNameAskUser:
					askUserInput := call.Input.AskUser
					if askUserInput == nil {
						slog.Error("ask user input not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_AskUser{
									AskUser: &v1.ToolCall_AskUserInput{
										Question: askUserInput.Question,
										Options:  askUserInput.Options,
									},
								},
							},
						},
					})
				case toolbase.ToolNameHandoff:
					handoffInput := call.Input.Handoff
					if handoffInput == nil {
						slog.Error("handoff input not set")
						continue
					}

					contentParts = append(contentParts, &v1.MessagePart{
						Data: &v1.MessagePart_ToolCall{
							ToolCall: &v1.ToolCall{
								ToolName: call.ToolName,
								Input: &v1.ToolCall_Handoff{
									Handoff: &v1.ToolCall_HandoffInput{
										RequestedAgent:  handoffInput.RequestedAgent,
										HandoverMessage: handoffInput.HandoverMessage,
									},
								},
							},
						},
					})
				}
			}
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
			kind := types.MessageBlockKindNativeToolCall
			if b.Tool == "code_interpreter" {
				kind = types.MessageBlockKindCodeInterpreterCall
			}
			messageBlocks = append(messageBlocks, types.MessageBlock{
				Kind:    kind,
				Payload: string(payload),
			})
		case *model.ToolResultBlock:
			payload, err := json.Marshal(b)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool result block: %w", err)
			}
			kind := types.MessageBlockKindNativeToolResult
			if b.Name == "code_interpreter" {
				kind = types.MessageBlockKindCodeInterpreterResult
			}
			messageBlocks = append(messageBlocks, types.MessageBlock{
				Kind:    kind,
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

