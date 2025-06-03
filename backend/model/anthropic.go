package model

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/uuid"
)

type AnthropicProvider struct {
	client *anthropic.Client
}

func SupportedAnthropicModels() []Model {
	return []Model{
		{
			ID:       uuid.MustParse("0195b4e2-45b6-76df-b208-f48b7b0d5f51"),
			Name:     "claude-3-7-sonnet-20250219",
			Provider: Anthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityComputerUse,
				CapabilityPromptCache,
				CapabilityExtendedThinking,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      3.0,
				Output:     15.0,
				CacheWrite: 3.75,
				CacheRead:  0.3,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e2-7d71-79e0-97da-3045fb1ffc3e"),
			Name:     "claude-3-5-sonnet-20241022",
			Provider: Anthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityComputerUse,
				CapabilityPromptCache,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      3.0,
				Output:     15.0,
				CacheWrite: 3.75,
				CacheRead:  0.3,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e2-a5df-736d-82ea-00f46db3dadc"),
			Name:     "claude-3-5-sonnet-20240620",
			Provider: Anthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityComputerUse,
				CapabilityPromptCache,
			},
			ContextWindow: 100000,
			Pricing: ModelPricing{
				Input:      3.0,
				Output:     15.0,
				CacheWrite: 3.75,
				CacheRead:  0.3,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e2-c741-724d-bb2a-3b0f7fdbc5f4"),
			Name:     "claude-3-5-haiku-20241022",
			Provider: Anthropic,
			Capabilities: []Capability{
				CapabilityPromptCache,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      0.8,
				Output:     4.0,
				CacheWrite: 1.0,
				CacheRead:  0.08,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e2-efd4-7c5c-a9a2-219318e0e181"),
			Name:     "claude-3-opus-20240229",
			Provider: Anthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityPromptCache,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      15.0,
				Output:     75.0,
				CacheWrite: 18.75,
				CacheRead:  1.5,
			},
		},
		{
			ID:       uuid.MustParse("0195b4e3-1da7-71af-ba34-6689aed6c4a2"),
			Name:     "claude-3-haiku-20240307",
			Provider: Anthropic,
			Capabilities: []Capability{
				CapabilityImage,
				CapabilityPromptCache,
			},
			ContextWindow: 200000,
			Pricing: ModelPricing{
				Input:      0.25,
				Output:     1.25,
				CacheWrite: 0.3,
				CacheRead:  0.03,
			},
		},
	}
}

func NewAnthropicProvider(apiKey string) (*AnthropicProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic API key is required")
	}

	provider := &AnthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}

	return provider, nil
}

func (p *AnthropicProvider) InvokeModel(ctx context.Context, model, systemPrompt string, messages []*Message, opts ...InvokeModelOption) (*Message, error) {
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	if systemPrompt == "" {
		return nil, fmt.Errorf("system prompt is required")
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}

	options := DefaultInvokeModelOptions()
	for _, opt := range opts {
		opt(options)
	}

	// convert to anthropic messages
	anthropicMessages := make([]anthropic.MessageParam, len(messages))
	for i, message := range messages {
		anthropicBlocks := make([]anthropic.ContentBlockParamUnion, len(message.Content))
		for j, b := range message.Content {
			switch block := b.(type) {
			case *TextBlock:
				anthropicBlocks[j] = anthropic.NewTextBlock(block.Text)
			case *ToolCallBlock:
				anthropicBlocks[j] = anthropic.NewToolUseBlockParam(block.ID, block.Tool, block.Args)
			case *ToolResultBlock:
				anthropicBlocks[j] = anthropic.NewToolResultBlock(block.ID, block.Result, !block.Succeeded)
			}
		}

		switch message.Source {
		case MessageSourceUser:
			anthropicMessages[i] = anthropic.NewUserMessage(anthropicBlocks...)
		case MessageSourceModel:
			anthropicMessages[i] = anthropic.NewAssistantMessage(anthropicBlocks...)
		}
	}

	// convert to anthropic tools
	var tools []anthropic.ToolUnionUnionParam
	for i, tool := range options.Tools {
		toolParam := anthropic.ToolParam{
			Name:        anthropic.F(tool.Name()),
			Description: anthropic.F(tool.Description()),
			InputSchema: anthropic.F(tool.Schema()),
		}

		if i == len(options.Tools)-1 {
			toolParam.CacheControl = anthropic.F(
				anthropic.CacheControlEphemeralParam{Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral)})
		}
		tools = append(tools, toolParam)
	}

	request := anthropic.MessageNewParams{
		Model:       anthropic.F(model),
		MaxTokens:   anthropic.F(int64(options.MaxTokens)),
		Temperature: anthropic.F(options.Temperature),
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

	if len(options.Tools) > 0 {
		request.ToolChoice = anthropic.F(anthropic.ToolChoiceUnionParam(anthropic.ToolChoiceAutoParam{Type: anthropic.F(anthropic.ToolChoiceAutoTypeAuto)}))
		request.Tools = anthropic.F(tools)
	}

	stream := p.client.Messages.NewStreaming(ctx, request)
	defer stream.Close()

	anthropicMessage := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		anthropicMessage.Accumulate(event)

		switch delta := event.Delta.(type) {
		case anthropic.ContentBlockDeltaEventDelta:
			if delta.Text != "" && options.StreamHandler != nil {
				options.StreamHandler(ctx, &Message{
					Source: MessageSourceModel,
					Content: []ContentBlock{
						&TextBlock{Text: delta.Text},
					},
				})
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

func (p *AnthropicProvider) GetModel(ctx context.Context, modelID uuid.UUID) (Model, error) {
	for _, model := range SupportedAnthropicModels() {
		if model.ID == modelID {
			return model, nil
		}
	}

	return Model{}, fmt.Errorf("model not supported")
}