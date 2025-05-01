package model

import (
	"context"
	"encoding/json"

	"github.com/furisto/construct/backend/tool"
)

type InvokeModelOptions struct {
	Messages      []Message
	Tools         []tool.NativeTool
	MaxTokens     int
	Temperature   float64
	StreamHandler func(ctx context.Context, message *Message)
}

func DefaultInvokeModelOptions() *InvokeModelOptions {
	return &InvokeModelOptions{
		Tools:       []tool.NativeTool{},
		MaxTokens:   8192,
		Temperature: 0.0,
	}
}

type InvokeModelOption func(*InvokeModelOptions)

func WithTools(tools ...tool.NativeTool) InvokeModelOption {
	return func(o *InvokeModelOptions) {
		o.Tools = tools
	}
}

func WithMaxTokens(maxTokens int) InvokeModelOption {
	return func(o *InvokeModelOptions) {
		o.MaxTokens = maxTokens
	}
}

func WithTemperature(temperature float64) InvokeModelOption {
	return func(o *InvokeModelOptions) {
		o.Temperature = temperature
	}
}

func WithStreamHandler(handler func(ctx context.Context, message *Message)) InvokeModelOption {
	return func(o *InvokeModelOptions) {
		o.StreamHandler = handler
	}
}

type ModelProvider interface {
	InvokeModel(ctx context.Context, model, prompt string, messages []Message, opts ...InvokeModelOption) (*ModelResponse, error)
}

type MessageSource string

const (
	MessageSourceUser  MessageSource = "user"
	MessageSourceModel MessageSource = "model"
)

type Message struct {
	Source  MessageSource
	Content []ContentBlock
}

func NewModelMessage(content []ContentBlock) *Message {
	return &Message{
		Source:  MessageSourceModel,
		Content: content,
	}
}

type ContentBlockType string

const (
	ContentBlockTypeText        ContentBlockType = "text"
	ContentBlockTypeToolRequest ContentBlockType = "tool_request"
	ContentBlockTypeToolResult  ContentBlockType = "tool_result"
)

type ContentBlock interface {
	Type() ContentBlockType
}

type TextBlock struct {
	Text string
}

func (t *TextBlock) Type() ContentBlockType {
	return ContentBlockTypeText
}

type ToolCallBlock struct {
	ID   string
	Tool string
	Args json.RawMessage
}

func (t *ToolCallBlock) Type() ContentBlockType {
	return ContentBlockTypeToolRequest
}

type ToolResultBlock struct {
	ID        string
	Name      string
	Result    string
	Succeeded bool
}

func (t *ToolResultBlock) Type() ContentBlockType {
	return ContentBlockTypeToolResult
}

type ModelResponse struct {
	Message *Message
	Usage   Usage
}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheWriteTokens int64
	CacheReadTokens  int64
}
