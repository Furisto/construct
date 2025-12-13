package codeact

import (
	"log/slog"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/communication"
	"github.com/furisto/construct/backend/tool/filesystem"
	"github.com/furisto/construct/backend/tool/subtask"
	"github.com/furisto/construct/backend/tool/system"
	"github.com/furisto/construct/backend/tool/types"
	"github.com/google/uuid"
	"github.com/grafana/sobek"
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

type ToolCall struct {
	ToolName string           `json:"tool_name"`
	Input    types.ToolInput  `json:"input"`
	Output   types.ToolOutput `json:"output"`
	Index    int              `json:"index"`
}

type ToolCallState struct {
	Calls []ToolCall
	Index int
}

func NewToolCallState() *ToolCallState {
	return &ToolCallState{
		Calls: []ToolCall{},
		Index: 0,
	}
}

func convertToFunctionCallInput(toolName string, input any) types.ToolInput {
	var result types.ToolInput

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
	case base.ToolNameSpawnTask:
		if v, ok := input.(*subtask.SpawnTaskInput); ok {
			result.SpawnTask = v
		}
	case base.ToolNameSendMessage:
		if v, ok := input.(*subtask.SendMessageInput); ok {
			result.SendMessage = v
		}
	case base.ToolNameAwaitTasks:
		if v, ok := input.(*subtask.AwaitTasksInput); ok {
			result.AwaitTasks = v
		}
	default:
		slog.Error("unknown tool name", "tool_name", toolName)
	}

	return result
}

func convertToFunctionCallOutput(toolName string, output any) types.ToolOutput {
	var result types.ToolOutput

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
	case base.ToolNameSpawnTask:
		if v, ok := output.(*subtask.SpawnTaskResult); ok {
			result.SpawnTask = v
		}
	case base.ToolNameSendMessage:
		if v, ok := output.(*subtask.SendMessageResult); ok {
			result.SendMessage = v
		}
	case base.ToolNameAwaitTasks:
		if v, ok := output.(*subtask.AwaitTasksResult); ok {
			result.AwaitTasks = v
		}
	default:
		slog.Error("unknown tool name", "tool_name", toolName)
	}

	return result
}

func DurableFunctionInterceptor(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		if tool.Name() != base.ToolNamePrint {
			callState, ok := GetValue[*ToolCallState](session, "function_call_state")
			if !ok {
				callState = NewToolCallState()
			}
			functionCall := ToolCall{
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
	EventBus *event.Bus
}

func NewToolEventPublisher(eventBus *event.Bus) *ToolEventPublisher {
	return &ToolEventPublisher{
		EventBus: eventBus,
	}
}

func (p *ToolEventPublisher) Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		if tool.Name() != base.ToolNamePrint {
			toolInput := p.publishToolCallEvent(session, tool, call.Arguments)
			result := inner(call)
			p.publishToolResultEvent(session, tool, toolInput)
			return result
		} else {
			return inner(call)
		}
	}
}

func (p *ToolEventPublisher) publishToolCallEvent(session *Session, tool Tool, arguments []sobek.Value) types.ToolInput {
	input, err := tool.Input(session, arguments)
	if err != nil {
		slog.Error("failed to get tool input", "error", err)
	}

	typedInput, err := types.ToolInputFromAny(input)
	if err != nil {
		slog.Error("failed to convert input to typed input", "error", err)
	}

	event.Publish(p.EventBus, types.ToolCallEvent{
		TaskID:   session.Task.ID,
		ToolName: tool.Name(),
		Input:    typedInput,
	})

	return typedInput
}

func (p *ToolEventPublisher) publishToolResultEvent(session *Session, tool Tool, toolInput types.ToolInput) {
	raw, ok := GetValue[any](session, "result")
	if !ok {
		slog.Error("failed to get tool result", "tool_name", tool.Name())
	}

	typedResult, err := types.ToolOutputFromAny(raw)
	if err != nil {
		slog.Error("failed to convert result to typed result", "error", err)
	}

	event.Publish(p.EventBus, types.ToolResultEvent{
		TaskID:   session.Task.ID,
		ToolName: tool.Name(),
		Input:    toolInput,
		Output:   typedResult,
	})
}
