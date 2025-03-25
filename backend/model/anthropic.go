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
	models map[uuid.UUID]Model
}

func NewAnthropicProvider(apiKey string) (*AnthropicProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic API key is required")
	}

	provider := &AnthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		models: make(map[uuid.UUID]Model),
	}

	models := []Model{
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
	for _, model := range models {
		provider.models[model.ID] = model
	}

	return provider, nil
}

func (p *AnthropicProvider) InvokeModel(ctx context.Context, model uuid.UUID, systemPrompt string, messages []Message, opts ...InvokeModelOption) (*ModelResponse, error) {
	if model == uuid.Nil {
		return nil, fmt.Errorf("model is required")
	}

	m, ok := p.models[model]
	if !ok {
		return nil, fmt.Errorf("model not supported")
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
			case *TextContentBlock:
				anthropicBlocks[j] = anthropic.NewTextBlock(block.Text)
			}
		}

		// if i == len(messages)-1 || i == prevUserMessageIndex {
		// 	block.CacheControl = anthropic.F(anthropic.CacheControlEphemeralParam{
		// 		Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral),
		// 	})
		// }

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
			Name:        anthropic.F(tool.Name),
			Description: anthropic.F(tool.Description),
			InputSchema: anthropic.F(tool.Schema),
		}

		if i == len(options.Tools)-1 {
			toolParam.CacheControl = anthropic.F(
				anthropic.CacheControlEphemeralParam{Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral)})
		}
		tools = append(tools, toolParam)
	}

	request := anthropic.MessageNewParams{
		Model:       anthropic.F(m.Name),
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
						&TextContentBlock{Text: delta.Text},
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
			content[i] = &TextContentBlock{
				Text: block.Text,
			}
		case anthropic.ToolUseBlock:
			content[i] = &ToolCallContentBlock{
				Name:  block.Name,
				Input: block.Input,
			}
		}
	}

	return &ModelResponse{
		Message: NewModelMessage(content),
		Usage: Usage{
			InputTokens:      anthropicMessage.Usage.InputTokens,
			OutputTokens:     anthropicMessage.Usage.OutputTokens,
			CacheWriteTokens: anthropicMessage.Usage.CacheCreationInputTokens,
			CacheReadTokens:  anthropicMessage.Usage.CacheReadInputTokens,
		},
	}, nil
}

// func (p *AnthropicProvider) ListModels(ctx context.Context) ([]Model, error) {
// 	resp, err := p.client.Models.List(ctx, anthropic.ModelListParams{})
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to list anthropic models: %w", err)
// 	}

// 	var models []Model
// 	for _, model := range resp.Data {
// 		models = append(models, Model{
// 			Name:     model.ID,
// 			Provider: "anthropic",
// 		})
// 	}

// 	return models, nil
// }

func (p *AnthropicProvider) ListModels(ctx context.Context) ([]Model, error) {
	models := make([]Model, 0, len(p.models))
	for _, model := range p.models {
		models = append(models, model)
	}
	return models, nil
}

func (p *AnthropicProvider) GetModel(ctx context.Context, modelID uuid.UUID) (Model, error) {
	model, ok := p.models[modelID]
	if !ok {
		return Model{}, fmt.Errorf("model not supported")
	}
	return model, nil
}
