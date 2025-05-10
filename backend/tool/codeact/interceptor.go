package codeact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"

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

type FunctionExecution struct {
	ToolName string
	Input    []string
	Output   string
}

func FunctionExecutionInterceptor(session *Session, tool Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		functionResult := FunctionExecution{
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
		exported, err := export(result)
		if err != nil {
			slog.Error("failed to export result", "error", err)
		}
		functionResult.Output = exported

		executions, ok := GetValue[[]FunctionExecution](session, "executions")
		if !ok {
			executions = []FunctionExecution{}
		}
		executions = append(executions, functionResult)
		SetValue(session, "executions", executions)
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
