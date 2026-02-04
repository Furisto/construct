package agent

import (
	tooltypes "github.com/furisto/construct/backend/tool/types"
)

// ProviderData contains provider-specific metadata for tool calls.
type ProviderData struct {
	Kind string `json:"kind"` // "anthropic", "openai", "gemini"
	ID   string `json:"id"`   // Provider's tool call ID
}

// ToolCall represents a tool invocation with typed input.
type ToolCall struct {
	Tool     string               `json:"tool"`
	Input    *tooltypes.ToolInput `json:"input,omitempty"`
	Provider *ProviderData        `json:"provider"`
}

// ToolResult represents a tool result with typed output.
type ToolResult struct {
	Tool      string                `json:"tool"`
	Output    *tooltypes.ToolOutput `json:"output,omitempty"`
	Succeeded bool                  `json:"succeeded"`
	Provider  *ProviderData         `json:"provider"`
}
