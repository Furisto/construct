package model

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/furisto/construct/backend/tool/native"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/shared"
)

// OpenAIChatCompletionService defines the interface for OpenAI chat completions
// to allow for mocking in tests.
type OpenAIChatCompletionService interface {
	NewStreaming(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk]
}

// openaiChatCompletionServiceAdapter adapts the real OpenAI client to the interface.
type openaiChatCompletionServiceAdapter struct {
	client *openai.Client
}

func (a *openaiChatCompletionServiceAdapter) NewStreaming(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	return a.client.Chat.Completions.NewStreaming(ctx, params)
}

type OpenAICompletionProvider struct {
	chatService OpenAIChatCompletionService
}

func NewOpenAICompletionProvider(apiKey string, opts ...ProviderOption) (*OpenAICompletionProvider, error) {
	logger := slog.With("component", "openai_provider")

	if apiKey == "" {
		logger.Error("openai API key is required")
		return nil, fmt.Errorf("openai API key is required")
	}
	logger.Debug("initializing OpenAI provider")

	providerOptions := DefaultProviderOptions("openai")
	for _, opt := range opts {
		opt(providerOptions)
	}

	options := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if providerOptions.URL != "" {
		logger.Debug("using custom OpenAI URL",
			"url", providerOptions.URL,
		)
		options = append(options, option.WithBaseURL(providerOptions.URL))
	}

	logger.Info("OpenAI provider initialized successfully")

	client := openai.NewClient(options...)
	return &OpenAICompletionProvider{
		chatService: &openaiChatCompletionServiceAdapter{client: &client},
	}, nil
}

// NewOpenAICompletionProviderWithService creates a provider with a custom chat service.
// This is primarily used for testing with mock implementations.
func NewOpenAICompletionProviderWithService(chatService OpenAIChatCompletionService) *OpenAICompletionProvider {
	return &OpenAICompletionProvider{
		chatService: chatService,
	}
}

func (p *OpenAICompletionProvider) InvokeModel(ctx context.Context, model, systemPrompt string, messages []*Message, opts ...InvokeModelOption) (*Message, error) {
	logger := slog.With(
		"component", "openai_provider",
		"model", model,
		"message_count", len(messages),
	)

	if err := p.validateInput(model, systemPrompt, messages); err != nil {
		logger.Error("validation failed", "error", err)
		return nil, err
	}

	options := DefaultOpenAIModelOptions()
	for _, opt := range opts {
		opt(options)
	}

	modelProfile, err := ensureModelProfile[*OpenAIModelProfile](options.ModelProfile)
	if err != nil {
		logger.Error("failed to ensure model profile", "error", err)
		return nil, err
	}

	openaiMessages, err := p.transformMessages(messages)
	if err != nil {
		logger.Error("failed to transform messages", "error", err)
		return nil, err
	}
	logger.Debug("messages transformed",
		"transformed_count", len(openaiMessages),
	)

	openaiTools := p.transformTools(options.Tools)
	toolChoice := "auto"
	if !modelProfile.EnableFunctionCalling {
		toolChoice = "none"
	}
	logger.Debug("tools transformed",
		"tool_count", len(openaiTools),
		"tool_choice", toolChoice,
	)

	invokeStart := time.Now()
	logger.Debug("invoking OpenAI API")

	stream := p.chatService.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model:               model,
		MaxCompletionTokens: openai.Int(modelProfile.MaxTokens),
		Messages:            openaiMessages,
		Tools:               openaiTools,
		ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String(toolChoice),
		},
		ParallelToolCalls: openai.Bool(modelProfile.ParallelToolCalls),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
	})

	var accumulator openai.ChatCompletionAccumulator
	for stream.Next() {
		chunk := stream.Current()
		accumulator.AddChunk(chunk)

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" && options.StreamCallback != nil {
				options.StreamCallback(ctx, choice.Delta.Content)
			}
		}
	}

	if err := stream.Err(); err != nil {
		logger.Error("openai stream error",
			"error", err,
			"duration_ms", time.Since(invokeStart).Milliseconds(),
		)
		return nil, err
	}

	var content []ContentBlock
	for _, choice := range accumulator.Choices {
		switch {
		case choice.Message.Content != "":
			content = append(content, &TextBlock{Text: choice.Message.Content})
		case choice.Message.ToolCalls != nil:
			for _, toolCall := range choice.Message.ToolCalls {
				content = append(content, &ToolCallBlock{ID: toolCall.ID, Tool: toolCall.Function.Name, Args: json.RawMessage(toolCall.Function.Arguments)})
			}
		}
	}

	cacheHitRatio := 0.0
	if accumulator.Usage.PromptTokens+accumulator.Usage.PromptTokensDetails.CachedTokens > 0 {
		cacheHitRatio = float64(accumulator.Usage.PromptTokensDetails.CachedTokens) / float64(accumulator.Usage.PromptTokens+accumulator.Usage.PromptTokensDetails.CachedTokens)
	}

	logger.Info("openai invocation successful",
		"input_tokens", accumulator.Usage.PromptTokens,
		"output_tokens", accumulator.Usage.CompletionTokens,
		"cache_read_tokens", accumulator.Usage.PromptTokensDetails.CachedTokens,
		"cache_hit_ratio", fmt.Sprintf("%.1f%%", cacheHitRatio*100),
		"duration_ms", time.Since(invokeStart).Milliseconds(),
	)

	return NewModelMessage(content, Usage{
		InputTokens:      accumulator.Usage.PromptTokens,
		OutputTokens:     accumulator.Usage.CompletionTokens,
		CacheWriteTokens: 0,
		CacheReadTokens:  accumulator.Usage.PromptTokensDetails.CachedTokens,
	}), nil
}

func (p *OpenAICompletionProvider) transformMessages(messages []*Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	openaiMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))

	for _, message := range messages {
		switch message.Source {
		case MessageSourceUser:
			var content []openai.ChatCompletionContentPartUnionParam
			for _, block := range message.Content {
				switch b := block.(type) {
				case *TextBlock:
					content = append(content, openai.ChatCompletionContentPartUnionParam{
						OfText: &openai.ChatCompletionContentPartTextParam{
							Text: b.Text,
						},
					})
				}
			}
			openaiMessages = append(openaiMessages, openai.UserMessage(content))

		case MessageSourceModel:
			var content openai.ChatCompletionAssistantMessageParamContentUnion
			var toolCalls []openai.ChatCompletionMessageToolCallParam
			for _, block := range message.Content {
				switch b := block.(type) {
				case *TextBlock:
					content.OfString = openai.String(b.Text)

				case *ToolCallBlock:
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
						ID: b.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      b.Tool,
							Arguments: string(b.Args),
						},
					})
				}
			}
			assistantMessage := openai.ChatCompletionAssistantMessageParam{
				Content:   content,
				ToolCalls: toolCalls,
			}
			openaiMessages = append(openaiMessages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMessage})

		case MessageSourceSystem:
			for _, block := range message.Content {
				switch b := block.(type) {
				case *ToolResultBlock:
					openaiMessages = append(openaiMessages, openai.ToolMessage(b.Result, b.ID))
				}
			}
		}
	}

	return openaiMessages, nil
}

func (p *OpenAICompletionProvider) transformTools(tools []native.Tool) []openai.ChatCompletionToolParam {
	openaiTools := make([]openai.ChatCompletionToolParam, 0, len(tools))

	for _, tool := range tools {
		openaiTools = append(openaiTools, openai.ChatCompletionToolParam{
			Type: "function",
			Function: shared.FunctionDefinitionParam{
				Name:        tool.Name(),
				Description: openai.String(tool.Description()),
				Parameters:  tool.Schema(),
			},
		})
	}

	return openaiTools
}

func (p *OpenAICompletionProvider) validateInput(model, systemPrompt string, messages []*Message) error {
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
