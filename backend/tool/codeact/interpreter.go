package codeact

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/grafana/sobek"
	"github.com/invopop/jsonschema"
	"github.com/spf13/afero"
)

type InterpreterArgs struct {
	Script string `json:"script"`
}

type InterpreterResult struct {
	ConsoleOutput string           `json:"console_output"`
	FunctionCalls []FunctionCall   `json:"function_calls"`
	ToolStats     map[string]int64 `json:"tool_stats"`
}

type Interpreter struct {
	Tools        []Tool
	Interceptors []Interceptor

	inputSchema map[string]any
}

func NewInterpreter(tools []Tool, interceptors []Interceptor) *Interpreter {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var args InterpreterArgs
	reflected := reflector.Reflect(args)
	inputSchema := map[string]any{
		"type":       "object",
		"properties": reflected.Properties,
	}

	return &Interpreter{
		Tools:        tools,
		Interceptors: interceptors,
		inputSchema:  inputSchema,
	}
}

func (c *Interpreter) Name() string {
	return "code_interpreter"
}

func (c *Interpreter) Description() string {
	return "Can be used to call tools using Javascript syntax. Write a complete javascript program and use only the functions that have been specified. If you use any other functions the tool call will fail."
}

func (c *Interpreter) Schema() map[string]any {
	return c.inputSchema
}

func (c *Interpreter) Run(ctx context.Context, fsys afero.Fs, input json.RawMessage) (string, error) {
	return "", nil
}

func (c *Interpreter) Interpret(ctx context.Context, fsys afero.Fs, input json.RawMessage, task *Task) (*InterpreterResult, error) {
	var args InterpreterArgs
	err := json.Unmarshal(input, &args)
	if err != nil {
		return nil, err
	}

	vm := sobek.New()
	vm.SetFieldNameMapper(sobek.TagFieldNameMapper("json", true))

	var stdout bytes.Buffer
	session := NewSession(ctx, task, vm, &stdout, &stdout, fsys)

	for _, tool := range c.Tools {
		vm.Set(tool.Name(), c.intercept(session, tool, tool.ToolHandler(session)))
	}

	done := make(chan error)
	go func() {
		select {
		case <-ctx.Done():
			vm.Interrupt("execution cancelled")
		case <-done:
		}
	}()

	_, err = vm.RunString(ensureStrictMode(args.Script))
	close(done)

	callState, ok := GetValue[*FunctionCallState](session, "function_call_state")
	if !ok {
		callState = NewFunctionCallState()
	}

	toolStats, ok := GetValue[map[string]int64](session, "tool_stats")
	if !ok {
		toolStats = make(map[string]int64)
	}

	return &InterpreterResult{
		ConsoleOutput: stdout.String(),
		FunctionCalls: callState.Calls,
		ToolStats:     toolStats,
	}, err
}

func (c *Interpreter) intercept(session *Session, toolName Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	wrapped := inner
	for _, interceptor := range c.Interceptors {
		wrapped = interceptor.Intercept(session, toolName, wrapped)
	}
	return wrapped
}

func ensureStrictMode(script string) string {
	if strings.HasPrefix(script, "use strict;") {
		return script
	}

	return "'use strict';\n" + script
}
