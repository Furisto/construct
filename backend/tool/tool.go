package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
	"github.com/spf13/afero"
)

type ErrorCode int32

const (
	PathIsNotAbsolute ErrorCode = iota + 1
	PathIsDirectory
	PathIsNotDirectory
	PermissionDenied
	FileNotFound
	DirectoryNotFound
	CannotStatFile
	GenericFileError
	Internal
	None
	InvalidArgument
)

func (e ErrorCode) String() string {
	switch e {
	case PathIsNotAbsolute:
		return "Path is not absolute"
	case PathIsDirectory:
		return "Path is a directory"
	case PathIsNotDirectory:
		return "Path is not a directory"
	case PermissionDenied:
		return "Permission denied"
	case FileNotFound:
		return "File not found"
	case DirectoryNotFound:
		return "Directory not found"
	case CannotStatFile:
		return "Cannot stat file"
	case GenericFileError:
		return "File error"
	case Internal:
		return "Internal error"
	}
	return ""
}

func (e ErrorCode) Suggestion() []string {
	switch e {
	case PathIsNotAbsolute:
		return []string{
			"Ensure you're using an absolute path starting with '/'.",
			"Use the list_files tool to get the absolute path of a file.",
		}
	case PathIsDirectory:
		return []string{
			"You must specify a file path, not a directory.",
		}
	case PathIsNotDirectory:
		return []string{
			"You must specify a directory path, not a file path.",
		}
	case PermissionDenied:
		return []string{
			"Ensure you have write permissions for the file.",
		}
	case FileNotFound:
		return []string{
			"Ensure the file exists using the list_files tool.",
		}
	case DirectoryNotFound:
		return []string{
			"Ensure the directory exists using the list_files tool.",
		}
	case CannotStatFile:
		return []string{
			"Verify that you have the permission to read the file.",
		}
	case GenericFileError:
		return []string{
			"Check if the provided details can give you more information about the cause of the error.",
			"Verify that you have the permission to read the file.",
		}
	case Internal:
		return []string{
			"An internal error occurred. This is a bug with the tool itself. Try to work around it.",
		}
	}
	return []string{}
}

const (
	GenericSuggestion = "Check the provided system error for more details"
)

type ToolError struct {
	Message     string
	Suggestions []string
	Details     map[string]any
}

func (e *ToolError) Error() string {
	return e.Message
}

func NewError(code ErrorCode, args ...any) *ToolError {
	if len(args)%2 != 0 {
		args = append(args, "MISSING")
	}

	details := make(map[string]any)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			key = fmt.Sprintf("arg%d", i)
		}
		details[key] = args[i+1]
	}

	suggestions := append([]string{GenericSuggestion}, code.Suggestion()...)
	return &ToolError{
		Message:     code.String(),
		Suggestions: suggestions,
		Details:     details,
	}
}

func NewCustomError(message string, suggestions []string, args ...any) *ToolError {
	if len(args)%2 != 0 {
		args = append(args, "MISSING")
	}

	details := make(map[string]any)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			key = fmt.Sprintf("arg%d", i)
		}
		details[key] = args[i+1]
	}

	suggestions = append(suggestions, GenericSuggestion)
	return &ToolError{
		Message:     message,
		Suggestions: suggestions,
		Details:     details,
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
