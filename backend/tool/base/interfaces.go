package base

import (
	"context"
	"errors"

	"github.com/spf13/afero"
)

// BaseTool defines the core interface that all base tool implementations must satisfy
type BaseTool interface {
	// Name returns the tool's identifier
	Name() string
	// Description returns the tool's description for documentation
	Description() string
	// Execute runs the tool with the given input and returns the result
	Execute(ctx context.Context, fs afero.Fs, input interface{}) (interface{}, error)
}

// FileSystemTool defines the interface for filesystem-related tools
type FileSystemTool interface {
	BaseTool
}

// SearchTool defines the interface for search-related tools
type SearchTool interface {
	BaseTool
}

// SystemTool defines the interface for system-related tools
type SystemTool interface {
	BaseTool
}

// CommunicationTool defines the interface for communication-related tools
type CommunicationTool interface {
	BaseTool
}

// Common error types for tool operations
var (
	ErrInvalidInput      = errors.New("invalid input")
	ErrFileNotFound      = errors.New("file not found")
	ErrPermissionDenied  = errors.New("permission denied")
	ErrPathNotAbsolute   = errors.New("path must be absolute")
	ErrPathIsDirectory   = errors.New("path is a directory")
	ErrDirectoryNotFound = errors.New("directory not found")
)

// ValidationError represents input validation errors
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
