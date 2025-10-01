package model

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/furisto/construct/backend/tool/native"
	"github.com/furisto/construct/shared/resilience"
	"github.com/prometheus/client_golang/prometheus"
)

type InvokeModelOptions struct {
	Tools          []native.Tool
	StreamCallback func(ctx context.Context, chunk string)
	RetryCallback  func(ctx context.Context, err error, nextRetry time.Duration)
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

func WithRetryCallback(handler func(ctx context.Context, err error, nextRetry time.Duration)) InvokeModelOption {
	return func(o *InvokeModelOptions) {
		o.RetryCallback = handler
	}
}

type ProviderOptions struct {
	URL            string
	RetryConfig    *resilience.RetryConfig
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
			MaxAttempts:       5,
			InitialDelay:      1 * time.Second,
			MaxDelay:          10 * time.Second,
			BackoffMultiplier: 2,
		},
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

type ProviderError struct {
	Provider   string
	RetryAfter time.Duration
	Err        error
	Kind       ProviderErrorKind
}

func NewProviderError(provider string, kind ProviderErrorKind, err error) *ProviderError {
	return &ProviderError{
		Provider: provider,
		Kind:     kind,
		Err:      err,
	}
}

func (pe *ProviderError) Message() string {
	switch pe.Kind {
	case ProviderErrorKindInvalidRequest:
		return "Invalid request format or content"
	case ProviderErrorKindRateLimitExceeded:
		if pe.RetryAfter > 0 {
			return fmt.Sprintf("Rate limit exceeded, retry after %s", pe.RetryAfter)
		}
		return "Rate limit exceeded"
	case ProviderErrorKindOverloaded:
		return "API temporarily overloaded"
	case ProviderErrorKindInternal:
		return "Internal server error"
	case ProviderErrorKindTimeout:
		return "Request timeout"
	case ProviderErrorKindCanceled:
		return "Request canceled"
	case ProviderErrorKindUnknown:
		return "Unknown error"
	default:
		return "Unknown error"
	}
}

func (pe *ProviderError) Retryable() (bool, time.Duration) {
	switch pe.Kind {
	case ProviderErrorKindRateLimitExceeded:
		return true, pe.RetryAfter
	case ProviderErrorKindOverloaded:
		return true, 20 * time.Second
	default:
		return false, 0
	}
}

func (pe *ProviderError) retryableInternal() bool {
	switch pe.Kind {
	case ProviderErrorKindOverloaded,
		ProviderErrorKindInternal,
		ProviderErrorKindTimeout:
		return true
	default:
		return false
	}
}

func (pe *ProviderError) Error() string {
	if pe.Err != nil {
		return fmt.Sprintf("%s: %s: %s", pe.Provider, pe.Message(), pe.Err.Error())
	}
	return fmt.Sprintf("%s: %s", pe.Provider, pe.Message())
}

func (pe *ProviderError) Unwrap() error {
	return pe.Err
}

type ProviderErrorKind string

const (
	ProviderErrorKindInvalidRequest    ProviderErrorKind = "invalid_request"
	ProviderErrorKindRateLimitExceeded ProviderErrorKind = "rate_limit_exceeded"
	ProviderErrorKindOverloaded        ProviderErrorKind = "overloaded"
	ProviderErrorKindInternal          ProviderErrorKind = "internal"
	ProviderErrorKindTimeout           ProviderErrorKind = "timeout"
	ProviderErrorKindCanceled          ProviderErrorKind = "canceled"
	ProviderErrorKindUnknown           ProviderErrorKind = "unknown"
)
