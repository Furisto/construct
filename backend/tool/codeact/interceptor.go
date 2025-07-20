package codeact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/stream"
	"github.com/google/uuid"
	"github.com/grafana/sobek"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Interceptor interface {
	Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value
}

type InterceptorFunc func(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value

func (i InterceptorFunc) Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return i(session, tool, inner)
}

var _ Interceptor = InterceptorFunc(nil)

type FunctionCall struct {
	ToolName string
	Input    []string
	Output   any
}

func FunctionCallLogInterceptor(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		functionResult := FunctionCall{
			ToolName: tool.Name(),
		}
		for _, arg := range call.Arguments {
			exported, err := export(arg)
			if err != nil {
				slog.Error("failed to export argument", "error", err)
			}
			functionResult.Input = append(functionResult.Input, exported)
		}

		result := inner(call)
		if tool.Name() != "print" {
			functionResult.Output = result.Export()
			executions, ok := GetValue[[]FunctionCall](session, "executions")
			if !ok {
				executions = []FunctionCall{}
			}
			executions = append(executions, functionResult)
			SetValue(session, "executions", executions)
		}

		return result
	}
}

func export(value sobek.Value) (string, error) {
	switch kind := value.(type) {
	case sobek.String:
		return kind.String(), nil
	case *sobek.Object:
		jsonObject, err := kind.MarshalJSON()
		if err != nil {
			return "", NewError(Internal, "failed to marshal object")
		}
		var prettyJSON bytes.Buffer
		err = json.Indent(&prettyJSON, jsonObject, "", "  ")
		if err != nil {
			return "", NewError(Internal, "failed to format object")
		} else {
			return prettyJSON.String(), nil
		}
	default:
		return "", NewError(Internal, fmt.Sprintf("unknown type: %T", kind))
	}
}

func ToolNameInterceptor(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		session.CurrentTool = tool.Name()
		res := inner(call)
		session.CurrentTool = ""
		return res
	}
}

type ToolEventPublisher struct {
	EventHub *stream.EventHub
}

func NewToolEventPublisher(eventHub *stream.EventHub) *ToolEventPublisher {
	return &ToolEventPublisher{
		EventHub: eventHub,
	}
}

func (p *ToolEventPublisher) Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		toolCall := convertArgumentsToProtoToolCall(tool.Name(), call.Arguments, session.VM)
		if toolCall != nil {
			fmt.Printf("toolCall: %+v\n", toolCall)
		}
		p.publishToolEvent(session.TaskID, toolCall, v1.MessageRole_MESSAGE_ROLE_ASSISTANT)

		result := inner(call)

		toolResult, err := convertResultToProtoToolResult(tool.Name(), result, session.VM)
		if err != nil {
			slog.Error("failed to convert result to proto tool result", "error", err)
		}
		if toolResult != nil {
			fmt.Printf("toolResult: %+v\n", toolResult)
		}
		p.publishToolEvent(session.TaskID, toolResult, v1.MessageRole_MESSAGE_ROLE_USER)

		return result
	}
}

func (p *ToolEventPublisher) publishToolEvent(taskID uuid.UUID, part *v1.MessagePart, role v1.MessageRole) {
	if part == nil {
		return
	}

	p.EventHub.Publish(taskID, &v1.SubscribeResponse{
		Message: &v1.Message{
			Metadata: &v1.MessageMetadata{
				CreatedAt: timestamppb.New(time.Now()),
				UpdatedAt: timestamppb.New(time.Now()),
				TaskId:    taskID.String(),
				Role:      role,
			},
			Spec: &v1.MessageSpec{
				Content: []*v1.MessagePart{
					part,
				},
			},
			Status: &v1.MessageStatus{
				ContentState: v1.ContentStatus_CONTENT_STATUS_COMPLETE,
			},
		},
	})
}

func convertArgumentsToProtoToolCall(toolName string, arguments []sobek.Value, vm *sobek.Runtime) *v1.MessagePart {
	toolCall := &v1.ToolCall{
		ToolName: toolName,
	}

	switch toolName {
	case "create_file":
		if len(arguments) >= 2 {
			toolCall.Input = &v1.ToolCall_CreateFile{
				CreateFile: &v1.ToolCall_CreateFileInput{
					Path:    arguments[0].String(),
					Content: arguments[1].String(),
				},
			}
		}
	case "edit_file":
		if len(arguments) >= 2 {
			path := arguments[0].String()
			diffsArg := arguments[1]

			var diffs []*v1.ToolCall_EditFileInput_DiffPair
			if diffsObj := diffsArg.ToObject(vm); diffsObj != nil {
				if lengthVal := diffsObj.Get("length"); lengthVal != nil {
					length := int(lengthVal.ToInteger())
					for i := 0; i < length; i++ {
						if diffVal := diffsObj.Get(fmt.Sprintf("%d", i)); diffVal != nil {
							if diffObj := diffVal.ToObject(vm); diffObj != nil {
								oldText := ""
								newText := ""
								if oldVal := diffObj.Get("old"); oldVal != nil {
									oldText = oldVal.String()
								}
								if newVal := diffObj.Get("new"); newVal != nil {
									newText = newVal.String()
								}
								diffs = append(diffs, &v1.ToolCall_EditFileInput_DiffPair{
									Old: oldText,
									New: newText,
								})
							}
						}
					}
				}
			}

			toolCall.Input = &v1.ToolCall_EditFile{
				EditFile: &v1.ToolCall_EditFileInput{
					Path:  path,
					Diffs: diffs,
				},
			}
		}
	case "execute_command":
		if len(arguments) >= 1 {
			toolCall.Input = &v1.ToolCall_ExecuteCommand{
				ExecuteCommand: &v1.ToolCall_ExecuteCommandInput{
					Command: arguments[0].String(),
				},
			}
		}
	case "find_file":
		if len(arguments) >= 1 {
			inputObj := arguments[0].ToObject(vm)
			input := &v1.ToolCall_FindFileInput{}

			if pattern := inputObj.Get("pattern"); pattern != nil {
				input.Pattern = pattern.String()
			}
			if path := inputObj.Get("path"); path != nil {
				input.Path = path.String()
			}
			if excludePattern := inputObj.Get("exclude_pattern"); excludePattern != nil {
				input.ExcludePattern = excludePattern.String()
			}
			if maxResults := inputObj.Get("max_results"); maxResults != nil {
				input.MaxResults = int32(maxResults.ToInteger())
			}

			toolCall.Input = &v1.ToolCall_FindFile{
				FindFile: input,
			}
		}
	case "grep":
		if len(arguments) >= 1 {
			inputObj := arguments[0].ToObject(vm)
			input := &v1.ToolCall_GrepInput{}

			if query := inputObj.Get("query"); query != nil {
				input.Query = query.String()
			}
			if path := inputObj.Get("path"); path != nil {
				input.Path = path.String()
			}
			if includePattern := inputObj.Get("include_pattern"); includePattern != nil {
				input.IncludePattern = includePattern.String()
			}
			if excludePattern := inputObj.Get("exclude_pattern"); excludePattern != nil {
				input.ExcludePattern = excludePattern.String()
			}
			if caseSensitive := inputObj.Get("case_sensitive"); caseSensitive != nil {
				input.CaseSensitive = caseSensitive.ToBoolean()
			}
			if maxResults := inputObj.Get("max_results"); maxResults != nil {
				input.MaxResults = int32(maxResults.ToInteger())
			}

			toolCall.Input = &v1.ToolCall_Grep{
				Grep: input,
			}
		}
	case "handoff":
		if len(arguments) >= 1 {
			agent := arguments[0].String()
			var handoverMessage string
			if len(arguments) > 1 && arguments[1] != sobek.Undefined() {
				handoverMessage = arguments[1].String()
			}

			toolCall.Input = &v1.ToolCall_Handoff{
				Handoff: &v1.ToolCall_HandoffInput{
					RequestedAgent:  agent,
					HandoverMessage: handoverMessage,
				},
			}
		}
	case "ask_user":
		if len(arguments) >= 1 {
			inputObj := arguments[0].ToObject(vm)
			input := &v1.ToolCall_AskUserInput{}

			if question := inputObj.Get("question"); question != nil {
				input.Question = question.String()
			}
			if options := inputObj.Get("options"); options != nil {
				if optionsObj := options.ToObject(vm); optionsObj != nil {
					if lengthVal := optionsObj.Get("length"); lengthVal != nil {
						length := int(lengthVal.ToInteger())
						for i := 0; i < length; i++ {
							if optionVal := optionsObj.Get(fmt.Sprintf("%d", i)); optionVal != nil {
								input.Options = append(input.Options, optionVal.String())
							}
						}
					}
				}
			}

			toolCall.Input = &v1.ToolCall_AskUser{
				AskUser: input,
			}
		}
	case "list_files":
		if len(arguments) >= 2 {
			toolCall.Input = &v1.ToolCall_ListFiles{
				ListFiles: &v1.ToolCall_ListFilesInput{
					Path:      arguments[0].String(),
					Recursive: arguments[1].ToBoolean(),
				},
			}
		}
	case "read_file":
		if len(arguments) >= 1 {
			toolCall.Input = &v1.ToolCall_ReadFile{
				ReadFile: &v1.ToolCall_ReadFileInput{
					Path: arguments[0].String(),
				},
			}
		}
	case "submit_report":
		if len(arguments) >= 1 {
			inputObj := arguments[0].ToObject(vm)
			input := &v1.ToolCall_SubmitReportInput{}

			if summary := inputObj.Get("summary"); summary != nil {
				input.Summary = summary.String()
			}
			if completed := inputObj.Get("completed"); completed != nil {
				input.Completed = completed.ToBoolean()
			}
			if deliverables := inputObj.Get("deliverables"); deliverables != nil {
				if deliverablesObj := deliverables.ToObject(vm); deliverablesObj != nil {
					if lengthVal := deliverablesObj.Get("length"); lengthVal != nil {
						length := int(lengthVal.ToInteger())
						for i := 0; i < length; i++ {
							if deliverableVal := deliverablesObj.Get(fmt.Sprintf("%d", i)); deliverableVal != nil {
								input.Deliverables = append(input.Deliverables, deliverableVal.String())
							}
						}
					}
				}
			}
			if nextSteps := inputObj.Get("next_steps"); nextSteps != nil {
				input.NextSteps = nextSteps.String()
			}

			toolCall.Input = &v1.ToolCall_SubmitReport{
				SubmitReport: input,
			}
		}
	default:
		return nil
	}

	return &v1.MessagePart{
		Data: &v1.MessagePart_ToolCall{
			ToolCall: toolCall,
		},
	}
}

// convertResultToProtoToolResult converts JavaScript function result to proper proto ToolResult
func convertResultToProtoToolResult(toolName string, result sobek.Value, vm *sobek.Runtime) (*v1.MessagePart, error) {
	toolResult := &v1.ToolResult{
		ToolName: toolName,
	}

	exported, err := export(result)
	if err != nil {
		return nil, fmt.Errorf("failed to export result: %w", err)
	}

	if result == sobek.Undefined() || result == sobek.Null() {
		switch toolName {
		case "handoff":
			return nil, nil
		default:
			return nil, errors.New("tool returned undefined result")
		}
	}

	switch toolName {
	case "create_file":
		if resultObj := result.ToObject(vm); resultObj != nil {
			overwritten := false
			if overwrittenVal := resultObj.Get("overwritten"); overwrittenVal != nil {
				overwritten = overwrittenVal.ToBoolean()
			}
			toolResult.Result = &v1.ToolResult_CreateFile{
				CreateFile: &v1.ToolResult_CreateFileResult{
					Overwritten: overwritten,
				},
			}
		}
	case "edit_file":
		if resultObj := result.ToObject(vm); resultObj != nil {
			editResult := &v1.ToolResult_EditFileResult{}

			if path := resultObj.Get("path"); path != nil {
				editResult.Path = path.String()
			}
			if patchInfo := resultObj.Get("patch_info"); patchInfo != nil {
				if patchObj := patchInfo.ToObject(vm); patchObj != nil {
					patch := &v1.ToolResult_EditFileResult_PatchInfo{}
					if patchVal := patchObj.Get("patch"); patchVal != nil {
						patch.Patch = patchVal.String()
					}
					if linesAdded := patchObj.Get("lines_added"); linesAdded != nil {
						patch.LinesAdded = int32(linesAdded.ToInteger())
					}
					if linesRemoved := patchObj.Get("lines_removed"); linesRemoved != nil {
						patch.LinesRemoved = int32(linesRemoved.ToInteger())
					}
					editResult.PatchInfo = patch
				}
			}

			toolResult.Result = &v1.ToolResult_EditFile{
				EditFile: editResult,
			}
		}
	case "execute_command":
		if resultObj := result.ToObject(vm); resultObj != nil {
			execResult := &v1.ToolResult_ExecuteCommandResult{}

			if stdout := resultObj.Get("stdout"); stdout != nil {
				execResult.Stdout = stdout.String()
			}
			if stderr := resultObj.Get("stderr"); stderr != nil {
				execResult.Stderr = stderr.String()
			}
			if exitCode := resultObj.Get("exitCode"); exitCode != nil {
				execResult.ExitCode = int32(exitCode.ToInteger())
			}
			if command := resultObj.Get("command"); command != nil {
				execResult.Command = command.String()
			}

			toolResult.Result = &v1.ToolResult_ExecuteCommand{
				ExecuteCommand: execResult,
			}
		}
	case "find_file":
		if resultObj := result.ToObject(vm); resultObj != nil {
			findResult := &v1.ToolResult_FindFileResult{}

			if files := resultObj.Get("files"); files != nil {
				if filesObj := files.ToObject(vm); filesObj != nil {
					if lengthVal := filesObj.Get("length"); lengthVal != nil {
						length := int(lengthVal.ToInteger())
						for i := 0; i < length; i++ {
							if fileVal := filesObj.Get(fmt.Sprintf("%d", i)); fileVal != nil {
								findResult.Files = append(findResult.Files, fileVal.String())
							}
						}
					}
				}
			}
			if totalFiles := resultObj.Get("total_files"); totalFiles != nil {
				findResult.TotalFiles = int32(totalFiles.ToInteger())
			}
			if truncatedCount := resultObj.Get("truncated_count"); truncatedCount != nil {
				findResult.TruncatedCount = int32(truncatedCount.ToInteger())
			}

			toolResult.Result = &v1.ToolResult_FindFile{
				FindFile: findResult,
			}
		}
	case "grep":
		if resultObj := result.ToObject(vm); resultObj != nil {
			grepResult := &v1.ToolResult_GrepResult{}

			if matches := resultObj.Get("matches"); matches != nil {
				if matchesObj := matches.ToObject(vm); matchesObj != nil {
					if lengthVal := matchesObj.Get("length"); lengthVal != nil {
						length := int(lengthVal.ToInteger())
						for i := 0; i < length; i++ {
							if matchVal := matchesObj.Get(fmt.Sprintf("%d", i)); matchVal != nil {
								if matchObj := matchVal.ToObject(vm); matchObj != nil {
									match := &v1.ToolResult_GrepResult_GrepMatch{}
									if filePath := matchObj.Get("file_path"); filePath != nil {
										match.FilePath = filePath.String()
									}
									if lineNumber := matchObj.Get("line_number"); lineNumber != nil {
										match.LineNumber = int32(lineNumber.ToInteger())
									}
									if lineContent := matchObj.Get("line_content"); lineContent != nil {
										match.LineContent = lineContent.String()
									}
									grepResult.Matches = append(grepResult.Matches, match)
								}
							}
						}
					}
				}
			}
			if totalMatches := resultObj.Get("total_matches"); totalMatches != nil {
				grepResult.TotalMatches = int32(totalMatches.ToInteger())
			}
			if searchedFiles := resultObj.Get("searched_files"); searchedFiles != nil {
				grepResult.SearchedFiles = int32(searchedFiles.ToInteger())
			}

			toolResult.Result = &v1.ToolResult_Grep{
				Grep: grepResult,
			}
		}
	case "list_files":
		if resultObj := result.ToObject(vm); resultObj != nil {
			listResult := &v1.ToolResult_ListFilesResult{}

			if path := resultObj.Get("path"); path != nil {
				listResult.Path = path.String()
			}
			if entries := resultObj.Get("entries"); entries != nil {
				if entriesObj := entries.ToObject(vm); entriesObj != nil {
					if lengthVal := entriesObj.Get("length"); lengthVal != nil {
						length := int(lengthVal.ToInteger())
						for i := 0; i < length; i++ {
							if entryVal := entriesObj.Get(fmt.Sprintf("%d", i)); entryVal != nil {
								if entryObj := entryVal.ToObject(vm); entryObj != nil {
									entry := &v1.ToolResult_ListFilesResult_DirectoryEntry{}
									if name := entryObj.Get("n"); name != nil {
										entry.Name = name.String()
									}
									if entryType := entryObj.Get("t"); entryType != nil {
										entry.Type = entryType.String()
									}
									if size := entryObj.Get("s"); size != nil {
										entry.Size = size.ToInteger()
									}
									listResult.Entries = append(listResult.Entries, entry)
								}
							}
						}
					}
				}
			}

			toolResult.Result = &v1.ToolResult_ListFiles{
				ListFiles: listResult,
			}
		}
	case "read_file":
		if resultObj := result.ToObject(vm); resultObj != nil {
			readResult := &v1.ToolResult_ReadFileResult{}

			if path := resultObj.Get("path"); path != nil {
				readResult.Path = path.String()
			}
			if content := resultObj.Get("content"); content != nil {
				readResult.Content = content.String()
			}

			toolResult.Result = &v1.ToolResult_ReadFile{
				ReadFile: readResult,
			}
		}
	case "submit_report":
		if resultObj := result.ToObject(vm); resultObj != nil {
			submitResult := &v1.ToolResult_SubmitReportResult{}

			if summary := resultObj.Get("summary"); summary != nil {
				submitResult.Summary = summary.String()
			}
			if completed := resultObj.Get("completed"); completed != nil {
				submitResult.Completed = completed.ToBoolean()
			}
			if deliverables := resultObj.Get("deliverables"); deliverables != nil {
				if deliverablesObj := deliverables.ToObject(vm); deliverablesObj != nil {
					if lengthVal := deliverablesObj.Get("length"); lengthVal != nil {
						length := int(lengthVal.ToInteger())
						for i := 0; i < length; i++ {
							if deliverableVal := deliverablesObj.Get(fmt.Sprintf("%d", i)); deliverableVal != nil {
								submitResult.Deliverables = append(submitResult.Deliverables, deliverableVal.String())
							}
						}
					}
				}
			}
			if nextSteps := resultObj.Get("next_steps"); nextSteps != nil {
				submitResult.NextSteps = nextSteps.String()
			}

			toolResult.Result = &v1.ToolResult_SubmitReport{
				SubmitReport: submitResult,
			}
		}
	default:
		// For unknown tools, just store the exported result as a string
		slog.Warn("unknown tool result type", "tool", toolName, "result", exported)
	}

	return &v1.MessagePart{
		Data: &v1.MessagePart_ToolResult{
			ToolResult: toolResult,
		},
	}, nil
}
