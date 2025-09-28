package model

import (
	"context"
	"encoding/json"
	"time"

	"github.com/furisto/construct/backend/tool/native"
	"github.com/furisto/construct/shared/resilience"
	"github.com/prometheus/client_golang/prometheus"
)

type InvokeModelOptions struct {
	Tools          []native.Tool
	StreamCallback func(ctx context.Context, chunk string)
	ModelProfile   ModelProfile
}

type InvokeModelOption func(*InvokeModelOptions)

func WithTools(tools ...native.Tool) InvokeModelOption {
	return func(o *InvokeModelOptions) {
		o.Tools = tools
	}
}

func WithModelProfile(profile ModelProfile) InvokeModelOption {
	return func(o *InvokeModelOptions) {
		o.ModelProfile = profile
	}
}

func WithStreamHandler(handler func(ctx context.Context, chunk string)) InvokeModelOption {
	return func(o *InvokeModelOptions) {
		o.StreamCallback = handler
	}
}

type ProviderOptions struct {
	URL            string
	RetryConfig    *resilience.RetryConfig
	RetryHooks     []resilience.RetryHook
	CircuitBreaker *resilience.CircuitBreaker
	Metrics        *prometheus.Registry
}

type ProviderOption func(*ProviderOptions)

func WithURL(url string) ProviderOption {
	return func(options *ProviderOptions) {
		options.URL = url
	}
}

func WithRetryConfig(retryConfig *resilience.RetryConfig) ProviderOption {
	return func(options *ProviderOptions) {
		options.RetryConfig = retryConfig
	}
}

func WithRetryHooks(retryHooks []resilience.RetryHook) ProviderOption {
	return func(options *ProviderOptions) {
		options.RetryHooks = retryHooks
	}
}

func WithCircuitBreaker(circuitBreaker *resilience.CircuitBreaker) ProviderOption {

	return func(options *ProviderOptions) {
		options.CircuitBreaker = circuitBreaker
	}
}

func WithMetrics(metrics *prometheus.Registry) ProviderOption {
	return func(o *ProviderOptions) {
		o.Metrics = metrics
	}
}

func DefaultProviderOptions(name string) *ProviderOptions {
	return &ProviderOptions{
		RetryConfig: &resilience.RetryConfig{
			MaxAttempts:        5,
			InitialDelay:       1 * time.Second,
			MaxDelay:           10 * time.Second,
			UseProviderBackoff: true,
			BackoffMultiplier:  2,
		},
		RetryHooks:     []resilience.RetryHook{},
		CircuitBreaker: resilience.NewCircuitBreaker(name, 5, 10*time.Second),
		Metrics:        prometheus.NewRegistry(),
	}
}

type ModelProvider interface {
	InvokeModel(ctx context.Context, model, prompt string, messages []*Message, opts ...InvokeModelOption) (*Message, error)
}

type MessageSource string

const (
	MessageSourceUser   MessageSource = "user"
	MessageSourceModel  MessageSource = "model"
	MessageSourceSystem MessageSource = "system"
)

type Message struct {
	Source  MessageSource  `json:"source"`
	Content []ContentBlock `json:"content"`
	Usage   Usage          `json:"usage"`
}

func NewModelMessage(content []ContentBlock, usage Usage) *Message {
	return &Message{
		Source:  MessageSourceModel,
		Content: content,
		Usage:   usage,
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
	ID   string          `json:"id"`
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
}

func (t *ToolCallBlock) Type() ContentBlockType {
	return ContentBlockTypeToolRequest
}

type ToolResultBlock struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Result    string `json:"result"`
	Succeeded bool   `json:"succeeded"`
}

func (t *ToolResultBlock) Type() ContentBlockType {
	return ContentBlockTypeToolResult
}

type Usage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
}
