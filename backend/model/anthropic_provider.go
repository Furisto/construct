package model

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/furisto/construct/backend/tool/native"
	"github.com/furisto/construct/shared/resilience"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/avast/retry-go/v4"
)

type AnthropicProvider struct {
	client         *anthropic.Client
	retryConfig    *resilience.RetryConfig
	retryHooks     []resilience.RetryHook
	circuitBreaker *resilience.CircuitBreaker
	metrics        *prometheus.Registry
}

func NewAnthropicProvider(apiKey string, opts ...ProviderOption) (*AnthropicProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic API key is required")
	}

	providerOptions := DefaultProviderOptions("anthropic")
	for _, opt := range opts {
		opt(providerOptions)
	}

	var clientOptions []option.RequestOption
	if providerOptions.URL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(providerOptions.URL))
	}

	provider := &AnthropicProvider{
		client:         anthropic.NewClient(clientOptions...),
		retryConfig:    providerOptions.RetryConfig,
		retryHooks:     providerOptions.RetryHooks,
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
		Model:       anthropic.F(model),
		MaxTokens:   anthropic.F(modelProfile.MaxTokens),
		Temperature: anthropic.F(modelProfile.Temperature),
		System: anthropic.F([]anthropic.TextBlockParam{
			{
				Type: anthropic.F(anthropic.TextBlockParamTypeText),
				Text: anthropic.F(systemPrompt),
				CacheControl: anthropic.F(anthropic.CacheControlEphemeralParam{
					Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral),
				}),
			},
		}),
		Messages: anthropic.F(anthropicMessages),
	}

	if len(anthropicTools) > 0 {
		request.ToolChoice = anthropic.F(anthropic.ToolChoiceUnionParam(anthropic.ToolChoiceAutoParam{Type: anthropic.F(anthropic.ToolChoiceAutoTypeAuto)}))
		request.Tools = anthropic.F(anthropicTools)
	}

	stream := p.client.Messages.NewStreaming(ctx, request)
	defer stream.Close()

	anthropicMessage := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		anthropicMessage.Accumulate(event)

		switch delta := event.Delta.(type) {
		case anthropic.ContentBlockDeltaEventDelta:
			if delta.Text != "" && options.StreamCallback != nil {
				options.StreamCallback(ctx, delta.Text)
			}
		}
	}

	if stream.Err() != nil {
		return nil, fmt.Errorf("failed to stream response: %w", stream.Err())
	}

	content := make([]ContentBlock, len(anthropicMessage.Content))
	for i, block := range anthropicMessage.Content {
		switch block := block.AsUnion().(type) {
		case anthropic.TextBlock:
			content[i] = &TextBlock{
				Text: block.Text,
			}
		case anthropic.ToolUseBlock:
			content[i] = &ToolCallBlock{
				ID:   block.ID,
				Tool: block.Name,
				Args: block.Input,
			}
		}
	}

	return NewModelMessage(content, Usage{
		InputTokens:      anthropicMessage.Usage.InputTokens,
		OutputTokens:     anthropicMessage.Usage.OutputTokens,
		CacheWriteTokens: anthropicMessage.Usage.CacheCreationInputTokens,
		CacheReadTokens:  anthropicMessage.Usage.CacheReadInputTokens,
	}), nil
}

func (p *AnthropicProvider) invokeWithRetry(ctx context.Context, callFn retry.RetryableFuncWithData[*Message]) (*Message, error) {
	msg, err := retry.DoWithData(callFn,
		retry.DelayType(retry.DelayTypeFunc(p.calculateRetryDelay)),
		retry.OnRetry(func(n uint, err error) {

		}),
		retry.RetryIf(p.isRetryableError),
		retry.Attempts(p.retryConfig.MaxAttempts),
		retry.MaxDelay(p.retryConfig.MaxDelay),
		retry.Context(ctx),
	)

	if options.CircuitBreaker != nil {
		options.CircuitBreaker.RecordResult(err)
	}

	totalDuration := time.Since(startTime)
	if err != nil {
		for _, hook := range options.RetryHooks {
			hook.OnRetryFailure(ctx, lastError, retryState.attempts, totalDuration)
		}
		return nil, fmt.Errorf("all retry attempts failed: %w", lastError)
	}

	for _, hook := range options.RetryHooks {
		hook.OnRetrySuccess(ctx, retryState.attempts, totalDuration)
	}

	return result, nil
}

func (p *AnthropicProvider) isRetryableError(err error) bool {
	if providerErr, ok := err.(*ProviderError); ok {
		return providerErr.ShouldRetry()
	}
	return true
}

func (p *AnthropicProvider) executeCall(ctx context.Context, model, systemPrompt string, messages []*Message, options *InvokeModelOptions) (*Message, error) {
	attemptStart := time.Now()

	// Execute the actual API call
	msg, err := fn()
	if err == nil {
		result = msg
		return nil
	}

	providerErr := p.parseError(err)
	lastError = providerErr

	// Record metrics
	p.metrics.RecordAttempt(attemptStart, providerErr)

	return providerErr

}

func (p *AnthropicProvider) calculateRetryDelay(attempt uint, err error, cfg *retry.Config) time.Duration {
	providerErr, ok := err.(*ProviderError)
	if !ok {
		// Network error - use exponential backoff
		state.strategy = "exponential_backoff"
		return p.exponentialBackoff(attempt, config)
	}

	// Provider gave us explicit retry timing
	if config.UseProviderBackoff && providerErr.RetryAfter > 0 {
		state.strategy = "provider_directed"
		// Add small jitter
		jitter := time.Duration(rand.Float64() * 100 * float64(time.Millisecond))
		return providerErr.RetryAfter + jitter
	}

	// Provider gave us a retry timestamp
	if config.UseProviderBackoff && !providerErr.RetryAt.IsZero() {
		state.strategy = "provider_timestamp"
		waitTime := time.Until(providerErr.RetryAt)
		if waitTime > 0 {
			return waitTime
		}
	}

	// Check if we previously used provider timing but it failed
	if state.strategy == "provider_directed" && providerErr.StatusCode == 503 {
		state.strategy = "backoff_after_provider_failed"
		state.providerDelayFailed = true
		// Use more aggressive backoff
		return p.exponentialBackoff(attempt*2, config)
	}

	// Default exponential backoff
	state.strategy = "exponential_backoff"
	return p.exponentialBackoff(attempt, config)
}

func (p *AnthropicProvider) exponentialBackoff(attempt uint, config *RetryConfig) time.Duration {
	delay := float64(config.InitialDelay)
	for i := uint(1); i < attempt; i++ {
		delay *= config.BackoffMultiplier
	}

	// Add jitter (±10%)
	jitter := (rand.Float64() - 0.5) * 0.2 * delay
	finalDelay := time.Duration(delay + jitter)

	if finalDelay > config.MaxDelay {
		finalDelay = config.MaxDelay
	}

	return finalDelay
}

func (p *AnthropicProvider) parseError(err error) *ProviderError {
	// Parse Anthropic-specific error format
	providerErr := &ProviderError{
		Provider:    "anthropic",
		IsRetryable: true,
	}

	// Check if it's an Anthropic API error
	if apiErr, ok := err.(*anthropic.Error); ok {
		providerErr.StatusCode = apiErr.StatusCode
		providerErr.Message = apiErr.Message

		// Parse Retry-After header if present
		if retryAfter := apiErr.Header.Get("Retry-After"); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				providerErr.RetryAfter = time.Duration(seconds) * time.Second
			}
		}

		// Anthropic-specific: check for overloaded status
		if apiErr.StatusCode == 529 {
			providerErr.ErrorCode = "overloaded"
			providerErr.RetryAfter = 10 * time.Second // Default for overloaded
		}
	}

	return providerErr
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
				textBlock := anthropic.NewTextBlock(block.Text)
				if (i == lastUserMessageIndex || i == secondToLastUserMessageIndex) && j == len(message.Content)-1 {
					textBlock.CacheControl = anthropic.F(anthropic.CacheControlEphemeralParam{
						Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral),
					})
				}
				anthropicBlocks[j] = textBlock
			case *ToolCallBlock:
				anthropicBlocks[j] = anthropic.NewToolUseBlockParam(block.ID, block.Tool, block.Args)
			case *ToolResultBlock:
				toolResultBlock := anthropic.NewToolResultBlock(block.ID, block.Result, !block.Succeeded)
				if (i == lastUserMessageIndex || i == secondToLastUserMessageIndex) && j == len(message.Content)-1 {
					toolResultBlock.CacheControl = anthropic.F(anthropic.CacheControlEphemeralParam{
						Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral),
					})
				}
				anthropicBlocks[j] = toolResultBlock
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

func (p *AnthropicProvider) transformTools(tools []native.Tool) ([]anthropic.ToolUnionUnionParam, error) {
	var anthropicTools []anthropic.ToolUnionUnionParam
	for i, tool := range tools {
		toolParam := anthropic.ToolParam{
			Name:        anthropic.F(tool.Name()),
			Description: anthropic.F(tool.Description()),
			InputSchema: anthropic.F(any(tool.Schema())),
		}

		if i == len(tools)-1 {
			toolParam.CacheControl = anthropic.F(
				anthropic.CacheControlEphemeralParam{Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral)})
		}
		anthropicTools = append(anthropicTools, toolParam)
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
