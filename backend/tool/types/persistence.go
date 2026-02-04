package types

// ProviderData contains provider-specific metadata for tool calls.
// This type is used for serializing/deserializing tool calls and results
// to/from the message persistence layer.
type ProviderData struct {
	Kind string `json:"kind"` // "anthropic", "openai", "gemini"
	ID   string `json:"id"`   // Provider's tool call ID
}

// ToolCall represents a tool invocation with typed input.
// Used for persisting tool calls in message content blocks.
type ToolCall struct {
	Provider *ProviderData `json:"provider"`
	Tool     string        `json:"tool"`
	Input    *ToolInput    `json:"input,omitempty"`
}

// ToolResult represents a tool result with typed output.
// Used for persisting tool results in message content blocks.
type ToolResult struct {
	Provider  *ProviderData `json:"provider"`
	Tool      string        `json:"tool"`
	Output    *ToolOutput   `json:"output,omitempty"`
	Succeeded bool          `json:"succeeded"`
}
