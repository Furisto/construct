package interpreter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"

	"github.com/furisto/construct/backend/tool"
	"github.com/grafana/sobek"
	"github.com/invopop/jsonschema"
	"github.com/spf13/afero"
)

type CodeInterpreterArgs struct {
	Script string `json:"script"`
}

type CodeInterpreterResult struct {
	ConsoleOutput      string
	FunctionExecutions []FunctionExecution
}

type CodeInterpreter struct {
	Tools        []tool.CodeActTool
	Interceptors []Interceptor

	inputSchema any
}

func NewCodeInterpreter(tools ...tool.CodeActTool) *CodeInterpreter {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var args CodeInterpreterArgs
	reflected := reflector.Reflect(args)
	inputSchema := map[string]interface{}{
		"type":       "object",
		"properties": reflected.Properties,
	}

	interceptors := []Interceptor{
		InterceptorFunc(FunctionExecutionInterceptor),
		InterceptorFunc(ToolNameInterceptor),
	}

	return &CodeInterpreter{
		Tools:        tools,
		Interceptors: interceptors,
		inputSchema:  inputSchema,
	}
}

func (c *CodeInterpreter) Name() string {
	return "code_interpreter"
}

func (c *CodeInterpreter) Description() string {
	return "Can be used to call tools using Javascript syntax. Write a complete javascript program and use only the functions that have been specified. If you use any other functions the tool call will fail."
}

func (c *CodeInterpreter) Schema() any {
	return c.inputSchema
}

func (c *CodeInterpreter) Run(ctx context.Context, fsys afero.Fs, input json.RawMessage) (string, error) {
	return "", nil
}

func (c *CodeInterpreter) Interpret(ctx context.Context, fsys afero.Fs, input json.RawMessage) (*CodeInterpreterResult, error) {
	var args CodeInterpreterArgs
	err := json.Unmarshal(input, &args)
	if err != nil {
		return nil, err
	}

	vm := sobek.New()
	vm.SetFieldNameMapper(sobek.TagFieldNameMapper("json", true))

	var stdout bytes.Buffer
	session := &tool.CodeActSession{
		VM:     vm,
		System: &stdout,
		FS:     fsys,
	}

	for _, tool := range c.Tools {
		vm.Set(tool.Name(), c.intercept(session, tool, tool.ToolCallback(session)))
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

	executions, ok := tool.GetValue[[]FunctionExecution](session, "executions")
	if !ok {
		executions = []FunctionExecution{}
	}

	return &CodeInterpreterResult{
		ConsoleOutput:      stdout.String(),
		FunctionExecutions: executions,
	}, err
}

func (c *CodeInterpreter) intercept(session *tool.CodeActSession, toolName tool.CodeActTool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		for _, interceptor := range c.Interceptors {
			inner = interceptor.Intercept(session, toolName, inner)
		}
		return inner(call)
	}
}
