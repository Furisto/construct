package model

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/furisto/construct/backend/tool/native"
	"github.com/furisto/construct/shared/resilience"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cenkalti/backoff/v5"
)

type AnthropicProvider struct {
	client         *anthropic.Client
	retryConfig    *resilience.RetryConfig
	circuitBreaker *resilience.CircuitBreaker
	metrics        *prometheus.Registry
}

func NewAnthropicProvider(apiKey string, opts ...ProviderOption) (*AnthropicProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic API key is required")
	}
	clientOptions := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}

	providerOptions := DefaultProviderOptions("anthropic")
	for _, opt := range opts {
		opt(providerOptions)
	}

	if providerOptions.URL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(providerOptions.URL))
	}

	client := anthropic.NewClient(clientOptions...)
	provider := &AnthropicProvider{
		client:         &client,
		retryConfig:    providerOptions.RetryConfig,
		circuitBreaker: providerOptions.CircuitBreaker,
		metrics:        providerOptions.Metrics,
	}

	return provider, nil
}

func (p *AnthropicProvider) InvokeModel(ctx context.Context, model, systemPrompt string, messages []*Message, opts ...InvokeModelOption) (*Message, error) {
	if err := p.validateInput(model, systemPrompt, messages); err != nil {
		return nil, err
	}

	options := defaultAnthropicInvokeOptions()
	for _, opt := range opts {
		opt(options)
	}

	modelProfile, err := ensureModelProfile[*AnthropicModelProfile](options.ModelProfile)
	if err != nil {
		return nil, err
	}

	anthropicMessages, err := p.transformMessages(messages)
	if err != nil {
		return nil, err
	}

	anthropicTools, err := p.transformTools(options.Tools)
	if err != nil {
		return nil, err
	}

	request := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: modelProfile.MaxTokens,
		System: []anthropic.TextBlockParam{
			{
				Text:         systemPrompt,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		},
		Messages: anthropicMessages,
	}

	if len(anthropicTools) > 0 {
		request.ToolChoice = anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}}
		request.Tools = anthropicTools
	}

	return p.invokeInternal(ctx, request, options)
}

func (p *AnthropicProvider) invokeInternal(ctx context.Context, request anthropic.MessageNewParams, options *InvokeModelOptions) (*Message, error) {
	retryOptions := []backoff.RetryOption{
		backoff.WithMaxTries(p.retryConfig.MaxAttempts),
		backoff.WithMaxElapsedTime(p.retryConfig.MaxDelay),
		backoff.WithBackOff(backoff.NewExponentialBackOff()),
		backoff.WithNotify(func(err error, next time.Duration) {
			options.RetryCallback(ctx, err, next)
		}),
	}

	return backoff.Retry(ctx, func() (*Message, error) {
		if !p.circuitBreaker.Allow() {
			return nil, backoff.Permanent(fmt.Errorf("too many errors from anthropic provider, circuit breaker open"))
		}

		stream := p.client.Messages.NewStreaming(ctx, request)
		defer stream.Close()

		anthropicMessage := anthropic.Message{}
		for stream.Next() {
			event := stream.Current()
			anthropicMessage.Accumulate(event)

			if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
				if event.Delta.Text != "" && options.StreamCallback != nil {
					options.StreamCallback(ctx, event.Delta.Text)
				}
			}
		}

		if stream.Err() != nil {
			slog.ErrorContext(ctx, "failed to invoke model", "error", stream.Err(), "provider", "anthropic")
			p.circuitBreaker.RecordResult(stream.Err())
			err := p.mapError(stream.Err())
			if err.retryableInternal() {
				return nil, stream.Err()
			}
			return nil, backoff.Permanent(err)
		}

		content := make([]ContentBlock, len(anthropicMessage.Content))
		for i, block := range anthropicMessage.Content {
			switch block.Type {
			case "text":
				content[i] = &TextBlock{
					Text: block.Text,
				}
			case "tool_use":
				content[i] = &ToolCallBlock{
					ID:   block.ID,
					Tool: block.Name,
					Args: block.Input,
				}
			}
		}

		p.circuitBreaker.RecordResult(nil)
		return NewModelMessage(content, Usage{
			InputTokens:      anthropicMessage.Usage.InputTokens,
			OutputTokens:     anthropicMessage.Usage.OutputTokens,
			CacheWriteTokens: anthropicMessage.Usage.CacheCreationInputTokens,
			CacheReadTokens:  anthropicMessage.Usage.CacheReadInputTokens,
		}), nil
	}, retryOptions...)
}

func (p *AnthropicProvider) mapError(err error) *ProviderError {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		var kind ProviderErrorKind

		switch apiErr.StatusCode {
		case 400, 401, 403, 404, 413:
			kind = ProviderErrorKindInvalidRequest
		case 429:
			kind = ProviderErrorKindRateLimitExceeded
		case 529:
			kind = ProviderErrorKindOverloaded
		default:
			if apiErr.StatusCode >= 500 && apiErr.StatusCode < 600 {
				kind = ProviderErrorKindInternal
			} else {
				kind = ProviderErrorKindUnknown
			}
		}

		providerErr := NewAnthropicProviderError(kind, err)

		if kind == ProviderErrorKindRateLimitExceeded {
			if retryAfter := apiErr.Response.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := time.ParseDuration(retryAfter + "s"); err == nil {
					providerErr.RetryAfter = seconds
				}
			}
		}

		return providerErr
	}

	return NewAnthropicProviderError(ProviderErrorKindUnknown, err)
}

func defaultAnthropicInvokeOptions() *InvokeModelOptions {
	return &InvokeModelOptions{
		Tools:          []native.Tool{},
		ModelProfile:   defaultAnthropicModelProfile(),
		StreamCallback: nil,
	}
}

func defaultAnthropicModelProfile() *AnthropicModelProfile {
	return &AnthropicModelProfile{
		MaxTokens:  8192,
		MaxRetries: 0,
	}
}

func (p *AnthropicProvider) transformMessages(messages []*Message) ([]anthropic.MessageParam, error) {
	var lastUserMessageIndex, secondToLastUserMessageIndex int = -1, -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Source == MessageSourceUser {
			if lastUserMessageIndex == -1 {
				lastUserMessageIndex = i
			} else if secondToLastUserMessageIndex == -1 {
				secondToLastUserMessageIndex = i
				break
			}
		}
	}

	anthropicMessages := make([]anthropic.MessageParam, len(messages))
	for i, message := range messages {
		anthropicBlocks := make([]anthropic.ContentBlockParamUnion, len(message.Content))
		for j, b := range message.Content {
			switch block := b.(type) {
			case *TextBlock:
				textBlockParam := anthropic.TextBlockParam{
					Text: block.Text,
				}
				if (i == lastUserMessageIndex || i == secondToLastUserMessageIndex) && j == len(message.Content)-1 {
					textBlockParam.CacheControl = anthropic.NewCacheControlEphemeralParam()
				}
				anthropicBlocks[j] = anthropic.ContentBlockParamUnion{OfText: &textBlockParam}
			case *ToolCallBlock:
				toolUseBlock := anthropic.ToolUseBlockParam{
					ID:    block.ID,
					Name:  block.Tool,
					Input: block.Args,
				}
				anthropicBlocks[j] = anthropic.ContentBlockParamUnion{OfToolUse: &toolUseBlock}
			case *ToolResultBlock:
				toolResultBlockParam := anthropic.ToolResultBlockParam{
					ToolUseID: block.ID,
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: block.Result}},
					},
					IsError: anthropic.Bool(!block.Succeeded),
				}
				if (i == lastUserMessageIndex || i == secondToLastUserMessageIndex) && j == len(message.Content)-1 {
					toolResultBlockParam.CacheControl = anthropic.NewCacheControlEphemeralParam()
				}
				anthropicBlocks[j] = anthropic.ContentBlockParamUnion{OfToolResult: &toolResultBlockParam}
			}
		}

		switch message.Source {
		case MessageSourceUser:
			anthropicMessages[i] = anthropic.NewUserMessage(anthropicBlocks...)
		case MessageSourceModel:
			anthropicMessages[i] = anthropic.NewAssistantMessage(anthropicBlocks...)
		case MessageSourceSystem:
			anthropicMessages[i] = anthropic.NewUserMessage(anthropicBlocks...)
		}
	}

	return anthropicMessages, nil
}

func (p *AnthropicProvider) transformTools(tools []native.Tool) ([]anthropic.ToolUnionParam, error) {
	var anthropicTools []anthropic.ToolUnionParam
	for i, tool := range tools {
		schema := tool.Schema()
		inputSchema := anthropic.ToolInputSchemaParam{
			ExtraFields: schema,
		}

		if props, ok := schema["properties"].(map[string]any); ok {
			inputSchema.Properties = props
		}
		if req, ok := schema["required"].([]any); ok {
			required := make([]string, len(req))
			for i, r := range req {
				if s, ok := r.(string); ok {
					required[i] = s
				}
			}
			inputSchema.Required = required
		}

		toolParam := anthropic.ToolParam{
			Name:        tool.Name(),
			Description: anthropic.String(tool.Description()),
			InputSchema: inputSchema,
		}

		if i == len(tools)-1 {
			toolParam.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{OfTool: &toolParam})
	}

	return anthropicTools, nil
}

func (p *AnthropicProvider) validateInput(model, systemPrompt string, messages []*Message) error {
	if model == "" {
		return fmt.Errorf("model is required")
	}

	if systemPrompt == "" {
		return fmt.Errorf("system prompt is required")
	}

	if len(messages) == 0 {
		return fmt.Errorf("at least one message is required")
	}

	return nil
}

func NewAnthropicProviderError(kind ProviderErrorKind, err error) *ProviderError {
	return NewProviderError("anthropic", kind, err)
}
