package codeact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/stream"
	"github.com/furisto/construct/backend/tool/communication"
	"github.com/furisto/construct/backend/tool/filesystem"
	"github.com/furisto/construct/backend/tool/system"
	"github.com/furisto/construct/shared"
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
		defer UnsetValue(session, "result")

		toolCall, err := convertArgumentsToProtoToolCall(tool, call.Arguments, session)
		if err != nil {
			slog.Error("failed to convert arguments to proto tool call", "error", err)
		}
		p.publishToolEvent(session.TaskID, toolCall, v1.MessageRole_MESSAGE_ROLE_ASSISTANT)

		result := inner(call)
		raw, ok := GetValue[any](session, "result")

		if ok {
			toolResult, err := convertResultToProtoToolResult(tool.Name(), raw)
			if err != nil {
				slog.Error("failed to convert result to proto tool result", "error", err)
			}
			if toolResult != nil {
				fmt.Printf("toolResult: %+v\n", toolResult)
			}
			p.publishToolEvent(session.TaskID, toolResult, v1.MessageRole_MESSAGE_ROLE_SYSTEM)
		}

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

func convertArgumentsToProtoToolCall(tooCall Tool, arguments []sobek.Value, session *Session) (*v1.MessagePart, error) {
	toolCall := &v1.ToolCall{
		ToolName: tooCall.Name(),
	}

	in, err := tooCall.Input(session, arguments)
	if err != nil {
		return nil, err
	}

	switch input := in.(type) {
	case *filesystem.CreateFileInput:
		toolCall.Input = &v1.ToolCall_CreateFile{
			CreateFile: &v1.ToolCall_CreateFileInput{
				Path:    input.Path,
				Content: input.Content,
			},
		}
	case *filesystem.EditFileInput:
		var diffs []*v1.ToolCall_EditFileInput_DiffPair
		for _, diff := range input.Diffs {
			diffs = append(diffs, &v1.ToolCall_EditFileInput_DiffPair{
				Old: diff.Old,
				New: diff.New,
			})
		}
		toolCall.Input = &v1.ToolCall_EditFile{
			EditFile: &v1.ToolCall_EditFileInput{
				Path:  input.Path,
				Diffs: diffs,
			},
		}
	case *system.ExecuteCommandInput:
		toolCall.Input = &v1.ToolCall_ExecuteCommand{
			ExecuteCommand: &v1.ToolCall_ExecuteCommandInput{
				Command: input.Command,
			},
		}
	case *filesystem.FindFileInput:
		toolCall.Input = &v1.ToolCall_FindFile{
			FindFile: &v1.ToolCall_FindFileInput{
				Pattern:        input.Pattern,
				Path:           input.Path,
				ExcludePattern: input.ExcludePattern,
				MaxResults:     int32(input.MaxResults),
			},
		}
	case *filesystem.GrepInput:
		toolCall.Input = &v1.ToolCall_Grep{
			Grep: &v1.ToolCall_GrepInput{
				Query:          input.Query,
				Path:           input.Path,
				IncludePattern: input.IncludePattern,
				ExcludePattern: input.ExcludePattern,
				CaseSensitive:  input.CaseSensitive,
				MaxResults:     int32(input.MaxResults),
			},
		}
	case *communication.HandoffInput:
		toolCall.Input = &v1.ToolCall_Handoff{
			Handoff: &v1.ToolCall_HandoffInput{
				RequestedAgent:  input.RequestedAgent,
				HandoverMessage: input.HandoverMessage,
			},
		}
	case *communication.AskUserInput:
		toolCall.Input = &v1.ToolCall_AskUser{
			AskUser: &v1.ToolCall_AskUserInput{
				Question: input.Question,
				Options:  input.Options,
			},
		}
	case *filesystem.ListFilesInput:
		toolCall.Input = &v1.ToolCall_ListFiles{
			ListFiles: &v1.ToolCall_ListFilesInput{
				Path:      input.Path,
				Recursive: input.Recursive,
			},
		}
	case *filesystem.ReadFileInput:
		toolCall.Input = &v1.ToolCall_ReadFile{
			ReadFile: &v1.ToolCall_ReadFileInput{
				Path: input.Path,
			},
		}
	case *communication.SubmitReportInput:
		toolCall.Input = &v1.ToolCall_SubmitReport{
			SubmitReport: &v1.ToolCall_SubmitReportInput{
				Summary:      input.Summary,
				Completed:    input.Completed,
				Deliverables: input.Deliverables,
				NextSteps:    input.NextSteps,
			},
		}
	case *communication.PrintInput:
		return nil, nil
	default:
		return nil, shared.Errorf(shared.ErrorSourceSystem, "unknown tool input type: %T", input)
	}

	return &v1.MessagePart{
		Data: &v1.MessagePart_ToolCall{
			ToolCall: toolCall,
		},
	}, nil
}

// convertResultToProtoToolResult converts tool result to proper proto ToolResult
func convertResultToProtoToolResult(toolName string, result any) (*v1.MessagePart, error) {
	toolResult := &v1.ToolResult{
		ToolName: toolName,
	}

	switch result := result.(type) {
	case *filesystem.CreateFileResult:
		toolResult.Result = &v1.ToolResult_CreateFile{
			CreateFile: &v1.ToolResult_CreateFileResult{
				Overwritten: result.Overwritten,
			},
		}
	case *filesystem.EditFileResult:
		editResult := &v1.ToolResult_EditFileResult{
			Path: result.Path,
		}
		if result.PatchInfo.Patch != "" {
			editResult.PatchInfo = &v1.ToolResult_EditFileResult_PatchInfo{
				Patch:        result.PatchInfo.Patch,
				LinesAdded:   int32(result.PatchInfo.LinesAdded),
				LinesRemoved: int32(result.PatchInfo.LinesRemoved),
			}
		}
		toolResult.Result = &v1.ToolResult_EditFile{
			EditFile: editResult,
		}
	case *system.ExecuteCommandResult:
		toolResult.Result = &v1.ToolResult_ExecuteCommand{
			ExecuteCommand: &v1.ToolResult_ExecuteCommandResult{
				Stdout:   result.Stdout,
				Stderr:   result.Stderr,
				ExitCode: int32(result.ExitCode),
				Command:  result.Command,
			},
		}
	case *filesystem.FindFileResult:
		toolResult.Result = &v1.ToolResult_FindFile{
			FindFile: &v1.ToolResult_FindFileResult{
				Files:          result.Files,
				TotalFiles:     int32(result.TotalFiles),
				TruncatedCount: int32(result.TruncatedCount),
			},
		}
	case *filesystem.GrepResult:
		var matches []*v1.ToolResult_GrepResult_GrepMatch
		for _, match := range result.Matches {
			matches = append(matches, &v1.ToolResult_GrepResult_GrepMatch{
				FilePath:    match.FilePath,
				LineNumber:  int32(match.LineNumber),
				LineContent: match.LineContent,
			})
		}
		toolResult.Result = &v1.ToolResult_Grep{
			Grep: &v1.ToolResult_GrepResult{
				Matches:       matches,
				TotalMatches:  int32(result.TotalMatches),
				SearchedFiles: int32(result.SearchedFiles),
			},
		}
	case *filesystem.ListFilesResult:
		var entries []*v1.ToolResult_ListFilesResult_DirectoryEntry
		for _, entry := range result.Entries {
			entries = append(entries, &v1.ToolResult_ListFilesResult_DirectoryEntry{
				Name: entry.Name,
				Type: entry.Type,
				Size: entry.Size,
			})
		}
		toolResult.Result = &v1.ToolResult_ListFiles{
			ListFiles: &v1.ToolResult_ListFilesResult{
				Path:    result.Path,
				Entries: entries,
			},
		}
	case *filesystem.ReadFileResult:
		toolResult.Result = &v1.ToolResult_ReadFile{
			ReadFile: &v1.ToolResult_ReadFileResult{
				Path:    result.Path,
				Content: result.Content,
			},
		}
	case *communication.SubmitReportResult:
		toolResult.Result = &v1.ToolResult_SubmitReport{
			SubmitReport: &v1.ToolResult_SubmitReportResult{
				Summary:      result.Summary,
				Completed:    result.Completed,
				Deliverables: result.Deliverables,
				NextSteps:    result.NextSteps,
			},
		}
	case nil:
		// Some tools like handoff don't return a result, only an error
		return nil, nil
	default:
		return nil, shared.Errorf(shared.ErrorSourceSystem, "unknown tool result type: %T", result)
	}

	return &v1.MessagePart{
		Data: &v1.MessagePart_ToolResult{
			ToolResult: toolResult,
		},
	}, nil
}
