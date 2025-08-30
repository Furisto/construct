package model

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/furisto/construct/backend/tool/native"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type OpenAICompletionProvider struct {
	client openai.Client
}

func NewOpenAICompletionProvider(apiKey string) (*OpenAICompletionProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openai API key is required")
	}
	return &OpenAICompletionProvider{client: openai.NewClient(option.WithAPIKey(apiKey))}, nil
}

func (p *OpenAICompletionProvider) InvokeModel(ctx context.Context, model, systemPrompt string, messages []*Message, opts ...InvokeModelOption) (*Message, error) {
	if err := p.validateInput(model, systemPrompt, messages); err != nil {
		return nil, err
	}

	options := DefaultOpenAIModelOptions()
	for _, opt := range opts {
		opt(options)
	}

	modelProfile, err := ensureModelProfile[*OpenAIModelProfile](options.ModelProfile)
	if err != nil {
		return nil, err
	}

	openaiMessages, err := p.transformMessages(messages)
	if err != nil {
		return nil, err
	}

	openaiTools := p.transformTools(options.Tools)
	toolChoice := "auto"
	if !modelProfile.EnableFunctionCalling {
		toolChoice = "none"
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
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
			if choice.Delta.Content != "" && options.StreamHandler != nil {
				options.StreamHandler(ctx, choice.Delta.Content)
			}
		}
	}

	if err := stream.Err(); err != nil {
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
