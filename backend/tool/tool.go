package tool

import (
	"context"
	"encoding/json"

	"github.com/invopop/jsonschema"
)


type ToolHandlerFunc[T any] func(ctx context.Context, input T) (string, error)

type Tool struct {
	Name        string
	Description string
	Schema      any
	Readonly    bool
	Handler     func(ctx context.Context, input json.RawMessage) (string, error)
}

func NewTool[T any](name string, description string, handler ToolHandlerFunc[T]) Tool {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
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
		Schema:      paramSchema,
		Handler:     genericToolHandler,
	}
}