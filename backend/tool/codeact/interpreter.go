package codeact

import (
	"bytes"
	"context"
	"encoding/json"
	"os"

	"github.com/grafana/sobek"
	"github.com/invopop/jsonschema"
	"github.com/spf13/afero"
)

type InterpreterArgs struct {
	Script string `json:"script"`
}

type InterpreterResult struct {
	ConsoleOutput      string
	FunctionExecutions []FunctionExecution
}

type Interpreter struct {
	Tools        []Tool
	Interceptors []Interceptor

	inputSchema any
}

func NewInterpreter(tools ...Tool) *Interpreter {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var args InterpreterArgs
	reflected := reflector.Reflect(args)
	inputSchema := map[string]interface{}{
		"type":       "object",
		"properties": reflected.Properties,
	}

	interceptors := []Interceptor{
		InterceptorFunc(FunctionExecutionInterceptor),
		InterceptorFunc(ToolNameInterceptor),
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

func (c *Interpreter) Schema() any {
	return c.inputSchema
}

func (c *Interpreter) Run(ctx context.Context, fsys afero.Fs, input json.RawMessage) (string, error) {
	return "", nil
}

func (c *Interpreter) Interpret(ctx context.Context, fsys afero.Fs, input json.RawMessage) (*InterpreterResult, error) {
	var args InterpreterArgs
	err := json.Unmarshal(input, &args)
	if err != nil {
		return nil, err
	}

	vm := sobek.New()
	vm.SetFieldNameMapper(sobek.TagFieldNameMapper("json", true))

	var stdout bytes.Buffer
	session := &Session{
		VM:     vm,
		System: &stdout,
		FS:     fsys,
	}

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

	os.WriteFile("/tmp/script.js", []byte(args.Script), 0644)
	_, err = vm.RunString(args.Script)
	close(done)

	executions, ok := GetValue[[]FunctionExecution](session, "executions")
	if !ok {
		executions = []FunctionExecution{}
	}

	return &InterpreterResult{
		ConsoleOutput:      stdout.String(),
		FunctionExecutions: executions,
	}, err
}

func (c *Interpreter) intercept(session *Session, toolName Tool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		for _, interceptor := range c.Interceptors {
			inner = interceptor.Intercept(session, toolName, inner)
		}
		return inner(call)
	}
}
