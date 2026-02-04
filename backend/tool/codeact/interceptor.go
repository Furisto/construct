package codeact

import (
	"log/slog"

	"github.com/furisto/construct/backend/tool/base"
	tooltypes "github.com/furisto/construct/backend/tool/types"
	"github.com/google/uuid"
	"github.com/grafana/sobek"
)

// EventPublisher is the interface for publishing tool events.
// This will be implemented by EventRouter in the new event streaming system.
type EventPublisher interface {
	PublishToolCall(taskID uuid.UUID, evt tooltypes.ToolCallEvent)
	PublishToolResult(taskID uuid.UUID, evt tooltypes.ToolResultEvent)
}

// NoopEventPublisher is a no-op implementation of EventPublisher.
// Useful for testing or when event publishing is not needed.
type NoopEventPublisher struct{}

func (NoopEventPublisher) PublishToolCall(taskID uuid.UUID, evt tooltypes.ToolCallEvent)     {}
func (NoopEventPublisher) PublishToolResult(taskID uuid.UUID, evt tooltypes.ToolResultEvent) {}

type Interceptor interface {
	Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value
}

type InterceptorFunc func(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value

func (i InterceptorFunc) Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return i(session, tool, inner)
}

var _ Interceptor = InterceptorFunc(nil)

// FunctionCall is an alias for tooltypes.FunctionCall for backward compatibility
type FunctionCall = tooltypes.FunctionCall

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

func convertToToolInput(input any) tooltypes.ToolInput {
	result, err := tooltypes.ToolInputFrom(input)
	if err != nil {
		slog.Error("failed to convert tool input", "error", err)
	}
	return result
}

func convertToToolOutput(output any) tooltypes.ToolOutput {
	result, err := tooltypes.ToolOutputFrom(output)
	if err != nil {
		slog.Error("failed to convert tool output", "error", err)
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
			functionCall.Input = convertToToolInput(input)

			result := inner(call)

			raw, ok := GetValue[any](session, "result")
			if !ok {
				slog.Error("failed to get result", "error", err)
			}

			functionCall.Output = convertToToolOutput(raw)
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
				toolInput, err := tooltypes.ToolInputFrom(input)
				if err != nil {
					slog.Error("failed to convert tool input", "error", err)
				} else {
					toolCallEvent := tooltypes.ToolCallEvent{
						Tool:  tool.Name(),
						Input: toolInput,
					}
					p.Publisher.PublishToolCall(session.Task.ID, toolCallEvent)
				}
			}

			result := inner(call)

			// Get tool result and publish tool result event
			raw, ok := GetValue[any](session, "result")
			if ok {
				toolOutput, err := tooltypes.ToolOutputFrom(raw)
				if err != nil {
					slog.Error("failed to convert tool output", "error", err)
				} else {
					toolResultEvent := tooltypes.ToolResultEvent{
						Tool:   tool.Name(),
						Output: toolOutput,
					}
					p.Publisher.PublishToolResult(session.Task.ID, toolResultEvent)
				}
			}
			return result
		}
		return inner(call)
	}
}
