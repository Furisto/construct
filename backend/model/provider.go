package model

import (
	"context"

	"github.com/furisto/construct/backend/toolbox"
	"github.com/google/uuid"
)

type InvokeModelOptions struct {
	Messages    []Message
	Tools       []toolbox.Tool
	MaxTokens   int
	Temperature float64
}

func DefaultInvokeModelOptions() *InvokeModelOptions {
	return &InvokeModelOptions{
		Tools:       []toolbox.Tool{},
		MaxTokens:   8192,
		Temperature: 0.0,
	}
}

type InvokeModelOption func(*InvokeModelOptions)

func WithTools(tools []toolbox.Tool) InvokeModelOption {
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
	Content string
}

type ContentBlockType string

const (
	ContentBlockTypeText     ContentBlockType = "text"
	ContentBlockTypeToolCall ContentBlockType = "tool_call"
)

type ContentBlock interface {
	Type() ContentBlockType
}

type TextBlock struct {
	Type ContentBlockType
	Text string
}

type ToolCallBlock struct {
	Type  ContentBlockType
	Name  string
	Input string
}

type ModelResponse struct {
	Blocks []ContentBlock
	Usage  Usage
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
