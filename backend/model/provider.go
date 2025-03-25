package model

import (
	"context"

	"github.com/furisto/construct/backend/tool"
	"github.com/google/uuid"
)

type InvokeModelOptions struct {
	Messages      []Message
	Tools         []tool.Tool
	MaxTokens     int
	Temperature   float64
	StreamHandler func(ctx context.Context, message *Message)
}

func DefaultInvokeModelOptions() *InvokeModelOptions {
	return &InvokeModelOptions{
		Tools:       []tool.Tool{},
		MaxTokens:   8192,
		Temperature: 0.0,
	}
}

type InvokeModelOption func(*InvokeModelOptions)

func WithTools(tools []tool.Tool) InvokeModelOption {
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
	InvokeModel(ctx context.Context, model uuid.UUID, prompt string, messages []Message, opts ...InvokeModelOption) (*ModelResponse, error)
}

type MessageSource string

const (
	MessageSourceUser  MessageSource = "user"
	MessageSourceModel MessageSource = "model"
	MessageSourceTool  MessageSource = "tool"
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
	ContentBlockTypeText     ContentBlockType = "text"
	ContentBlockTypeToolCall ContentBlockType = "tool_call"
)

type ContentBlock interface {
	Type() ContentBlockType
}

type TextContentBlock struct {
	Text string
}

func (t *TextContentBlock) Type() ContentBlockType {
	return ContentBlockTypeText
}

type ToolCallContentBlock struct {
	Name  string
	Input string
}

func (t *ToolCallContentBlock) Type() ContentBlockType {
	return ContentBlockTypeToolCall
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

type ToolCall struct {
	Name string
}
