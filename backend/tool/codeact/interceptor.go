package codeact

import (
	"log/slog"

	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/communication"
	"github.com/furisto/construct/backend/tool/filesystem"
	"github.com/furisto/construct/backend/tool/system"
	"github.com/furisto/construct/backend/tool/web"
	"github.com/google/uuid"
	"github.com/grafana/sobek"
)

// ToolCallEvent represents a tool call for the event streaming system.
type ToolCallEvent struct {
	ID    string    `json:"id"`
	Tool  string    `json:"tool"`
	Input ToolInput `json:"input"`
}

// ToolResultEvent represents a tool result for the event streaming system.
type ToolResultEvent struct {
	ID     string     `json:"id"`
	Tool   string     `json:"tool"`
	Output ToolOutput `json:"output"`
}

// ToolInput contains the typed input for a tool call.
type ToolInput struct {
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

// ToolOutput contains the typed output for a tool result.
type ToolOutput struct {
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

// EventPublisher is the interface for publishing tool events.
// This will be implemented by EventRouter in the new event streaming system.
type EventPublisher interface {
	PublishToolCall(taskID uuid.UUID, event ToolCallEvent)
	PublishToolResult(taskID uuid.UUID, event ToolResultEvent)
}

// NoopEventPublisher is a no-op implementation of EventPublisher.
// Used temporarily during the transition from MessageHub to EventRouter.
type NoopEventPublisher struct{}

func (NoopEventPublisher) PublishToolCall(taskID uuid.UUID, event ToolCallEvent)   {}
func (NoopEventPublisher) PublishToolResult(taskID uuid.UUID, event ToolResultEvent) {}

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
	Publisher EventPublisher
}

func NewToolEventPublisher(publisher EventPublisher) *ToolEventPublisher {
	return &ToolEventPublisher{
		Publisher: publisher,
	}
}

func (p *ToolEventPublisher) Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		if tool.Name() != base.ToolNamePrint {
			// Get tool input and publish tool call event
			input, err := tool.Input(session, call.Arguments)
			if err != nil {
				slog.Error("failed to get tool input", "error", err)
			} else {
				toolCallEvent := ToolCallEvent{
					Tool:  tool.Name(),
					Input: convertToToolInput(tool.Name(), input),
				}
				p.Publisher.PublishToolCall(session.Task.ID, toolCallEvent)
			}

			result := inner(call)

			// Get tool result and publish tool result event
			raw, ok := GetValue[any](session, "result")
			if ok {
				toolResultEvent := ToolResultEvent{
					Tool:   tool.Name(),
					Output: convertToToolOutput(tool.Name(), raw),
				}
				p.Publisher.PublishToolResult(session.Task.ID, toolResultEvent)
			}
			return result
		}
		return inner(call)
	}
}

// convertToToolInput converts a tool input to the ToolInput domain type.
func convertToToolInput(toolName string, input any) ToolInput {
	var result ToolInput
	switch v := input.(type) {
	case *filesystem.CreateFileInput:
		result.CreateFile = v
	case *filesystem.EditFileInput:
		result.EditFile = v
	case *system.ExecuteCommandInput:
		result.ExecuteCommand = v
	case *filesystem.FindFileInput:
		result.FindFile = v
	case *filesystem.GrepInput:
		result.Grep = v
	case *filesystem.ListFilesInput:
		result.ListFiles = v
	case *filesystem.ReadFileInput:
		result.ReadFile = v
	case *communication.SubmitReportInput:
		result.SubmitReport = v
	case *communication.AskUserInput:
		result.AskUser = v
	case *communication.HandoffInput:
		result.Handoff = v
	case *web.FetchInput:
		result.Fetch = v
	}
	return result
}

// convertToToolOutput converts a tool result to the ToolOutput domain type.
func convertToToolOutput(toolName string, output any) ToolOutput {
	var result ToolOutput
	switch v := output.(type) {
	case *filesystem.CreateFileResult:
		result.CreateFile = v
	case *filesystem.EditFileResult:
		result.EditFile = v
	case *system.ExecuteCommandResult:
		result.ExecuteCommand = v
	case *filesystem.FindFileResult:
		result.FindFile = v
	case *filesystem.GrepResult:
		result.Grep = v
	case *filesystem.ListFilesResult:
		result.ListFiles = v
	case *filesystem.ReadFileResult:
		result.ReadFile = v
	case *communication.SubmitReportResult:
		result.SubmitReport = v
	case *communication.AskUserResult:
		result.AskUser = v
	case *web.FetchResult:
		result.Fetch = v
	}
	return result
}

