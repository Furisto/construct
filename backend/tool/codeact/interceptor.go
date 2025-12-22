package codeact

import (
	"log/slog"
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/communication"
	"github.com/furisto/construct/backend/tool/filesystem"
	"github.com/furisto/construct/backend/tool/system"
	"github.com/furisto/construct/backend/tool/web"
	"github.com/furisto/construct/shared"
	"github.com/google/uuid"
	"github.com/grafana/sobek"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type EventHub interface {
	Publish(taskID uuid.UUID, message *v1.SubscribeResponse)
}

type Interceptor interface {
	Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value
}

type InterceptorFunc func(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value

func (i InterceptorFunc) Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return i(session, tool, inner)
}

var _ Interceptor = InterceptorFunc(nil)

type FunctionCallInput struct {
	CreateFile     *filesystem.CreateFileInput      `json:"create_file,omitempty"`
	EditFile       *filesystem.EditFileInput        `json:"edit_file,omitempty"`
	ExecuteCommand *system.ExecuteCommandInput      `json:"execute_command,omitempty"`
	FindFile       *filesystem.FindFileInput        `json:"find_file,omitempty"`
	Grep           *filesystem.GrepInput            `json:"grep,omitempty"`
	ListFiles      *filesystem.ListFilesInput       `json:"list_files,omitempty"`
	ReadFile       *filesystem.ReadFileInput        `json:"read_file,omitempty"`
	SubmitReport   *communication.SubmitReportInput `json:"submit_report,omitempty"`
	AskUser        *communication.AskUserInput      `json:"ask_user,omitempty"`
	Handoff        *communication.HandoffInput      `json:"handoff,omitempty"`
	Fetch          *web.FetchInput                  `json:"fetch,omitempty"`
}

type FunctionCallOutput struct {
	CreateFile     *filesystem.CreateFileResult      `json:"create_file,omitempty"`
	EditFile       *filesystem.EditFileResult        `json:"edit_file,omitempty"`
	ExecuteCommand *system.ExecuteCommandResult      `json:"execute_command,omitempty"`
	FindFile       *filesystem.FindFileResult        `json:"find_file,omitempty"`
	Grep           *filesystem.GrepResult            `json:"grep,omitempty"`
	ListFiles      *filesystem.ListFilesResult       `json:"list_files,omitempty"`
	ReadFile       *filesystem.ReadFileResult        `json:"read_file,omitempty"`
	SubmitReport   *communication.SubmitReportResult `json:"submit_report,omitempty"`
	AskUser        *communication.AskUserResult      `json:"ask_user,omitempty"`
	Fetch          *web.FetchResult                  `json:"fetch,omitempty"`
}

type FunctionCall struct {
	ToolName string             `json:"tool_name"`
	Input    FunctionCallInput  `json:"input"`
	Output   FunctionCallOutput `json:"output"`
	Index    int                `json:"index"`
}

type FunctionCallState struct {
	Calls []FunctionCall
	Index int
}

func NewFunctionCallState() *FunctionCallState {
	return &FunctionCallState{
		Calls: []FunctionCall{},
		Index: 0,
	}
}

func convertToFunctionCallInput(toolName string, input any) FunctionCallInput {
	var result FunctionCallInput

	switch toolName {
	case base.ToolNameCreateFile:
		if v, ok := input.(*filesystem.CreateFileInput); ok {
			result.CreateFile = v
		}
	case base.ToolNameEditFile:
		if v, ok := input.(*filesystem.EditFileInput); ok {
			result.EditFile = v
		}
	case base.ToolNameExecuteCommand:
		if v, ok := input.(*system.ExecuteCommandInput); ok {
			result.ExecuteCommand = v
		}
	case base.ToolNameFindFile:
		if v, ok := input.(*filesystem.FindFileInput); ok {
			result.FindFile = v
		}
	case base.ToolNameGrep:
		if v, ok := input.(*filesystem.GrepInput); ok {
			result.Grep = v
		}
	case base.ToolNameListFiles:
		if v, ok := input.(*filesystem.ListFilesInput); ok {
			result.ListFiles = v
		}
	case base.ToolNameReadFile:
		if v, ok := input.(*filesystem.ReadFileInput); ok {
			result.ReadFile = v
		}
	case base.ToolNameSubmitReport:
		if v, ok := input.(*communication.SubmitReportInput); ok {
			result.SubmitReport = v
		}
	case base.ToolNameAskUser:
		if v, ok := input.(*communication.AskUserInput); ok {
			result.AskUser = v
		}
	case base.ToolNameHandoff:
		if v, ok := input.(*communication.HandoffInput); ok {
			result.Handoff = v
		}
	case base.ToolNameFetch:
		if v, ok := input.(*web.FetchInput); ok {
			result.Fetch = v
		}
	default:
		slog.Error("unknown tool name", "tool_name", toolName)
	}

	return result
}

func convertToFunctionCallOutput(toolName string, output any) FunctionCallOutput {
	var result FunctionCallOutput

	switch toolName {
	case base.ToolNameCreateFile:
		if v, ok := output.(*filesystem.CreateFileResult); ok {
			result.CreateFile = v
		}
	case base.ToolNameEditFile:
		if v, ok := output.(*filesystem.EditFileResult); ok {
			result.EditFile = v
		}
	case base.ToolNameExecuteCommand:
		if v, ok := output.(*system.ExecuteCommandResult); ok {
			result.ExecuteCommand = v
		}
	case base.ToolNameFindFile:
		if v, ok := output.(*filesystem.FindFileResult); ok {
			result.FindFile = v
		}
	case base.ToolNameGrep:
		if v, ok := output.(*filesystem.GrepResult); ok {
			result.Grep = v
		}
	case base.ToolNameListFiles:
		if v, ok := output.(*filesystem.ListFilesResult); ok {
			result.ListFiles = v
		}
	case base.ToolNameReadFile:
		if v, ok := output.(*filesystem.ReadFileResult); ok {
			result.ReadFile = v
		}
	case base.ToolNameSubmitReport:
		if v, ok := output.(*communication.SubmitReportResult); ok {
			result.SubmitReport = v
		}
	case base.ToolNameAskUser:
		if v, ok := output.(*communication.AskUserResult); ok {
			result.AskUser = v
		}
	case base.ToolNameFetch:
		if v, ok := output.(*web.FetchResult); ok {
			result.Fetch = v
		}
	default:
		slog.Error("unknown tool name", "tool_name", toolName)
	}

	return result
}

func DurableFunctionInterceptor(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		if tool.Name() != base.ToolNamePrint {
			callState, ok := GetValue[*FunctionCallState](session, "function_call_state")
			if !ok {
				callState = NewFunctionCallState()
			}
			functionCall := FunctionCall{
				ToolName: tool.Name(),
				Index:    callState.Index,
			}

			input, err := tool.Input(session, call.Arguments)
			if err != nil {
				slog.Error("failed to get tool input", "error", err)
			}
			functionCall.Input = convertToFunctionCallInput(tool.Name(), input)

			result := inner(call)

			raw, ok := GetValue[any](session, "result")
			if !ok {
				slog.Error("failed to get result", "error", err)
			}

			functionCall.Output = convertToFunctionCallOutput(tool.Name(), raw)
			callState.Calls = append(callState.Calls, functionCall)
			callState.Index++
			SetValue(session, "function_call_state", callState)

			return result
		}

		return inner(call)
	}
}

func ToolStatisticsInterceptor(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		toolStats, ok := GetValue[map[string]int64](session, "tool_stats")
		if !ok {
			toolStats = make(map[string]int64)
		}
		if tool.Name() != base.ToolNamePrint {
			toolStats[tool.Name()]++
			SetValue(session, "tool_stats", toolStats)
		}
		return inner(call)
	}
}

func ResetTemporarySessionValuesInterceptor(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		UnsetValue(session, "result")
		return inner(call)
	}
}

type ToolEventPublisher struct {
	EventHub EventHub
}

func NewToolEventPublisher(eventHub EventHub) *ToolEventPublisher {
	return &ToolEventPublisher{
		EventHub: eventHub,
	}
}

func (p *ToolEventPublisher) Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		if tool.Name() != base.ToolNamePrint {
			toolCall, err := convertArgumentsToProtoToolCall(tool, call.Arguments, session)
			if err != nil {
				slog.Error("failed to convert arguments to proto tool call", "error", err)
			}
			p.publishToolEvent(session.Task.ID, toolCall, v1.MessageRole_MESSAGE_ROLE_ASSISTANT)

			result := inner(call)
			raw, ok := GetValue[any](session, "result")

			if ok {
				toolResult, err := convertResultToProtoToolResult(tool.Name(), raw)
				if err != nil {
					slog.Error("failed to convert result to proto tool result", "error", err)
				}
				p.publishToolEvent(session.Task.ID, toolResult, v1.MessageRole_MESSAGE_ROLE_SYSTEM)
			}
			return result
		} else {
			return inner(call)
		}
	}
}

func (p *ToolEventPublisher) publishToolEvent(taskID uuid.UUID, part *v1.MessagePart, role v1.MessageRole) {
	if part == nil {
		return
	}

	p.EventHub.Publish(taskID, &v1.SubscribeResponse{
		Event: &v1.SubscribeResponse_Message{
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
		readFile := &v1.ToolCall_ReadFile{
			ReadFile: &v1.ToolCall_ReadFileInput{
				Path: input.Path,
			},
		}
		if input.StartLine != nil {
			readFile.ReadFile.StartLine = int32(*input.StartLine)
		}
		if input.EndLine != nil {
			readFile.ReadFile.EndLine = int32(*input.EndLine)
		}
		toolCall.Input = readFile
	case *communication.SubmitReportInput:
		toolCall.Input = &v1.ToolCall_SubmitReport{
			SubmitReport: &v1.ToolCall_SubmitReportInput{
				Summary:      input.Summary,
				Completed:    input.Completed,
				Deliverables: input.Deliverables,
				NextSteps:    input.NextSteps,
			},
		}
	case *web.FetchInput:
		toolCall.Input = &v1.ToolCall_Fetch{
			Fetch: &v1.ToolCall_FetchInput{
				Url:     input.URL,
				Headers: input.Headers,
				Timeout: int32(input.Timeout),
			},
		}
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
				FilePath: match.FilePath,
				Value:    match.Value,
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
	case *web.FetchResult:
		toolResult.Result = &v1.ToolResult_Fetch{
			Fetch: &v1.ToolResult_FetchResult{
				Url:       result.URL,
				Title:     result.Title,
				Content:   result.Content,
				ByteSize:  int64(result.ByteSize),
				Truncated: result.Truncated,
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
