package tool

import (
	"context"
	"encoding/json"
	"io"

	"github.com/grafana/sobek"
	"github.com/invopop/jsonschema"
)

type CodeActToolCallback func(session CodeActSession) func(call sobek.FunctionCall) sobek.Value

type CodeActSession interface {
	VM() *sobek.Runtime
	Stdout() io.Writer
}

type CodeActTool interface {
	Name() string
	Description() string
	ToolCallback(session CodeActSession) func(call sobek.FunctionCall) sobek.Value
}

type onDemandTool struct {
	name        string
	description string
	handler     CodeActToolCallback
}

func (t *onDemandTool) Name() string {
	return t.name
}

func (t *onDemandTool) Description() string {
	return t.description
}

func (t *onDemandTool) ToolCallback(session CodeActSession) func(call sobek.FunctionCall) sobek.Value {
	return t.handler(session)
}

func NewOnDemandTool(name, description string, handler CodeActToolCallback) CodeActTool {
	return &onDemandTool{
		name:        name,
		description: description,
		handler:     handler,
	}
}

type ToolHandler[T any] func(ctx context.Context, input T) (string, error)

type ToolOptions struct {
	Readonly   bool
	Categories []string
}

func DefaultToolOptions() *ToolOptions {
	return &ToolOptions{
		Readonly:   false,
		Categories: []string{},
	}
}

type ToolOption func(*ToolOptions)

func WithReadonly(readonly bool) ToolOption {
	return func(o *ToolOptions) {
		o.Readonly = readonly
	}
}

func WithAdditionalCategory(category string) ToolOption {
	return func(o *ToolOptions) {
		o.Categories = append(o.Categories, category)
	}
}

type Tool struct {
	Name        string
	Description string
	Categories  []string
	Schema      any
	Readonly    bool
	Handler     any
}

func NewTool[T any](name, description, category string, handler ToolHandler[T], opts ...ToolOption) Tool {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}

	options := DefaultToolOptions()
	for _, opt := range opts {
		opt(options)
	}

	var toolInput T
	inputSchema := reflector.Reflect(toolInput)
	paramSchema := map[string]interface{}{
		"type":       "object",
		"properties": inputSchema.Properties,
	}

	if len(inputSchema.Required) > 0 {
		paramSchema["required"] = inputSchema.Required
	}

	genericToolHandler := func(ctx context.Context, input json.RawMessage) (string, error) {
		var toolInput T
		err := json.Unmarshal(input, &toolInput)
		if err != nil {
			return "", err
		}
		return handler(ctx, toolInput)
	}

	return Tool{
		Name:        name,
		Description: description,
		Categories:  options.Categories,
		Schema:      paramSchema,
		Readonly:    options.Readonly,
		Handler:     genericToolHandler,
	}
}
