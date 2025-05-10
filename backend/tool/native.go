package tool

import (
	"context"
	"encoding/json"

	"github.com/invopop/jsonschema"
	"github.com/spf13/afero"
)

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

type NativeTool interface {
	Name() string
	Description() string
	Schema() any
	Run(ctx context.Context, fs afero.Fs, input json.RawMessage) (string, error)
}

func NewTool[T any](name, description, category string, handler ToolHandler[T], opts ...ToolOption) NativeTool {
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

	// genericToolHandler := func(ctx context.Context, input json.RawMessage) (string, error) {
	// 	var toolInput T
	// 	err := json.Unmarshal(input, &toolInput)
	// 	if err != nil {
	// 		return "", err
	// 	}
	// 	return handler(ctx, toolInput)
	// }

	return nil
}
