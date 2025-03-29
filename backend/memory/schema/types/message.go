package types

type MessageContentBlockType string

const (
	MessageContentBlockTypeText MessageContentBlockType = "text"
)

type MessageContent struct {
	Blocks []MessageContentBlock `json:"blocks"`
}

type MessageContentBlock struct {
	Type MessageContentBlockType `json:"type"`
	Text string                  `json:"text"`
}

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

func (r MessageRole) Values() []string {
	return []string{
		string(MessageRoleUser),
		string(MessageRoleAssistant),
	}
}

type MessageUsage struct {
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	Cost             float64 `json:"cost"`
}
