package types

type MessageBlockKind string

const (
	MessageBlockKindText                  MessageBlockKind = "text"
	MessageBlockKindNativeToolCall        MessageBlockKind = "native_tool_call"
	MessageBlockKindNativeToolResult      MessageBlockKind = "native_tool_result"
	MessageBlockKindCodeActToolCall       MessageBlockKind = "code_act_tool_call"
	MessageBlockKindCodeActToolResult     MessageBlockKind = "code_act_tool_result"
	MessageBlockKindCodeInterpreterCall   MessageBlockKind = "code_interpreter_call"
	MessageBlockKindCodeInterpreterResult MessageBlockKind = "code_interpreter_result"
)

type MessageContent struct {
	Blocks []MessageBlock `json:"blocks"`
}

type MessageBlock struct {
	Kind    MessageBlockKind `json:"kind"`
	Payload string           `json:"payload"`
}

type MessageSource string

const (
	MessageSourceUser      MessageSource = "user"
	MessageSourceAssistant MessageSource = "assistant"
	MessageSourceSystem    MessageSource = "system"
)

func (r MessageSource) Values() []string {
	return []string{
		string(MessageSourceUser),
		string(MessageSourceAssistant),
		string(MessageSourceSystem),
	}
}

type MessageUsage struct {
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	Cost             float64 `json:"cost"`
}
