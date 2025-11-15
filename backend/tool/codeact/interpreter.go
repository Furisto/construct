package codeact

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/furisto/construct/shared"
	"github.com/grafana/sobek"
	"github.com/invopop/jsonschema"
	"github.com/spf13/afero"
)

type InterpreterInput struct {
	Script string `json:"script"`
}

type InterpreterOutput struct {
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
	var args InterpreterInput
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

func (c *Interpreter) Interpret(ctx context.Context, fsys afero.Fs, input json.RawMessage, task *Task) (*InterpreterOutput, error) {
	logger := slog.With(
		"component", "code_interpreter",
		"task_id", task.ID,
	)

	interpretStart := time.Now()

	var args InterpreterInput
	err := json.Unmarshal(input, &args)
	if err != nil {
		logger.Error("failed to unmarshal interpreter input", "error", err)
		return nil, err
	}

	scriptLines := strings.Count(args.Script, "\n") + 1
	logger.Debug("script execution started",
		"script_lines", scriptLines,
	)

	vm := sobek.New()
	vm.SetFieldNameMapper(sobek.TagFieldNameMapper("json", true))

	var stdout bytes.Buffer
	session := NewSession(ctx, task, vm, &stdout, &stdout, fsys, &shared.DefaultCommandRunner{})

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

	if err != nil {
		err = c.handleScriptError(err)
		logger.Error("script execution failed", "error", err)
	}

	callState, ok := GetValue[*FunctionCallState](session, "function_call_state")
	if !ok {
		callState = NewFunctionCallState()
	}

	toolStats, ok := GetValue[map[string]int64](session, "tool_stats")
	if !ok {
		toolStats = make(map[string]int64)
	}

	consoleOutput := stdout.String()
	logger.Info("script execution completed",
		"duration_ms", time.Since(interpretStart).Milliseconds(),
		"tool_call_count", len(callState.Calls),
		"console_output_size", len(consoleOutput),
		"tool_stats", toolStats,
	)

	return &InterpreterOutput{
		ConsoleOutput: consoleOutput,
		FunctionCalls: callState.Calls,
		ToolStats:     toolStats,
	}, err
}

func (c *Interpreter) handleScriptError(err error) error {
	exception, ok := err.(*sobek.Exception)
	if !ok {
		return err
	}

	if exception.Unwrap() != nil {
		return exception.Unwrap()
	}

	return err
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
