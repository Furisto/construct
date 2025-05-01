package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/furisto/construct/backend/tool"
	"github.com/grafana/sobek"
	"github.com/invopop/jsonschema"
	"github.com/spf13/afero"
)

type CodeInterpreterArgs struct {
	Script string `json:"script"`
}

type CodeInterpreter struct {
	Tools        []tool.CodeActTool
	Interceptors []CodeInterpreterInterceptor

	inputSchema any
}

func NewCodeInterpreter(tools []tool.CodeActTool, interceptors ...CodeInterpreterInterceptor) *CodeInterpreter {
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

	slices.Reverse(interceptors)
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

type CodeInterpreterResult struct {
	ConsoleOutput   string
	FunctionResults []FunctionResult
}

type FunctionResult struct {
	ToolName string
	Input    []string
	Output   string
}

func (c *CodeInterpreter) Run(ctx context.Context, fsys afero.Fs, input json.RawMessage) (string, error) {
	var args CodeInterpreterArgs
	err := json.Unmarshal(input, &args)
	if err != nil {
		return "", err
	}

	vm := sobek.New()
	vm.SetFieldNameMapper(sobek.TagFieldNameMapper("json", true))

	var stdout bytes.Buffer
	session := tool.CodeActSession{
		VM:     vm,
		System: &stdout,
		User:   &BlockWriter{},
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

	return stdout.String(), err
}

type CodeInterpreterInterceptor interface {
	Intercept(session tool.CodeActSession, tool tool.CodeActTool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value
}

type InputOutputInterceptor struct {
	results []FunctionResult
}

func NewInputOutputInterceptor() *InputOutputInterceptor {
	return &InputOutputInterceptor{}
}

func (r *InputOutputInterceptor) Results() []FunctionResult {
	return r.results
}

func (r *InputOutputInterceptor) Intercept(session tool.CodeActSession, tool tool.CodeActTool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		functionResult := FunctionResult{
			ToolName: tool.Name(),
		}
		for _, arg := range call.Arguments {
			exported, err := r.export(arg)
			if err != nil {
				slog.Error("failed to export argument", "error", err)
			}
			functionResult.Input = append(functionResult.Input, exported)
		}

		result := inner(call)
		exported, err := r.export(result)
		if err != nil {
			slog.Error("failed to export result", "error", err)
		}
		functionResult.Output = exported
		r.results = append(r.results, functionResult)
		return result
	}
}

func (r *InputOutputInterceptor) export(value sobek.Value) (string, error) {
	switch kind := value.(type) {
	case sobek.String:
		return kind.String(), nil
	case *sobek.Object:
		jsonObject, err := kind.MarshalJSON()
		if err != nil {
			return "", tool.NewToolError("failed to marshal object", "This is probably due to a bug in the tool implementation.")
		}
		var prettyJSON bytes.Buffer
		err = json.Indent(&prettyJSON, jsonObject, "", "  ")
		if err != nil {
			return "", tool.NewToolError("failed to format object", "This is probably due to a bug in the tool implementation.")
		} else {
			return prettyJSON.String(), nil
		}
	default:
		return "", tool.NewToolError(fmt.Sprintf("unknown type: %T", kind), "This is probably due to a bug in the tool implementation.")
	}
}

var _ CodeInterpreterInterceptor = (*InputOutputInterceptor)(nil)

func (c *CodeInterpreter) intercept(session tool.CodeActSession, toolName tool.CodeActTool, inner func(sobek.FunctionCall) sobek.Value) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		for _, interceptor := range c.Interceptors {
			inner = interceptor.Intercept(session, toolName, inner)
		}
		return inner(call)
	}
}

type BlockWriter struct {
	blocks []string
}

func (w *BlockWriter) Write(p []byte) (n int, err error) {
	w.blocks = append(w.blocks, string(p))
	return len(p), nil
}
