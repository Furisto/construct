package codeact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/stream"
	"github.com/grafana/sobek"
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

type FunctionResultPublisher struct {
	EventHub *stream.EventHub
}

func (p *FunctionResultPublisher) Intercept(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		result := inner(call)

		arguments := make(map[string]string)
		for _, arg := range call.Arguments {
			exported, err := export(arg)
			if err != nil {
				slog.Error("failed to export argument", "error", err)
			}
			arguments[arg.ExportType().Name()] = exported
		}

		exported, err := export(result)
		if err != nil {
			slog.Error("failed to export result", "error", err)
		}
		p.EventHub.Publish(session.TaskID, &v1.SubscribeResponse{
			Message: &v1.Message{
				Spec: &v1.MessageSpec{
					Content: []*v1.MessagePart{
						{
							Data: &v1.MessagePart_ToolResult_{
								ToolResult: &v1.MessagePart_ToolResult{
									ToolName:  v1.ToolName_UNSPECIFIED,
									Arguments: arguments,
									Result:    exported,
									Error:     "",
								},
							},
						},
					},
				},
			},
		})
		return result
	}
}
