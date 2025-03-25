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

	o := &InvokeModelOptions{}
	for _, opt := range opts {
		opt(o)
	}

	prevUserMessageIndex := -1
	for i := len(messages) - 2; i >= 0; i-- {
		if messages[i].Source == MessageSourceUser {
			prevUserMessageIndex = i
			break
		}
	}

	// convert to anthropic messages
	anthropicMessages := make([]anthropic.MessageParam, len(messages))
	for i, message := range messages {
		switch message.Source {
		case MessageSourceUser:
			block := anthropic.NewTextBlock(message.Content)
			if i == len(messages)-1 || i == prevUserMessageIndex {
				block.CacheControl = anthropic.F(anthropic.CacheControlEphemeralParam{
					Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral),
				})
			}
			anthropicMessages[i] = anthropic.NewUserMessage(block)
		case MessageSourceModel:
			anthropicMessages[i] = anthropic.NewAssistantMessage(anthropic.NewTextBlock(message.Content))
		}
	}

	// convert to anthropic tools
	var tools []anthropic.ToolUnionUnionParam
	for i, tool := range o.Tools {
		toolParam := anthropic.ToolParam{
			Name:        anthropic.F(tool.Name),
			Description: anthropic.F(tool.Description),
		}

		if i == len(o.Tools)-1 {
			toolParam.CacheControl = anthropic.F(
				anthropic.CacheControlEphemeralParam{Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral)})
		}
		tools = append(tools, toolParam)
	}

	stream := p.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:       anthropic.F(m.Name),
		MaxTokens:   anthropic.F(int64(o.MaxTokens)),
		Temperature: anthropic.F(o.Temperature),
		System: anthropic.F([]anthropic.TextBlockParam{
			{
				Type: anthropic.F(anthropic.TextBlockParamTypeText),
				Text: anthropic.F(systemPrompt),
				CacheControl: anthropic.F(anthropic.CacheControlEphemeralParam{
					Type: anthropic.F(anthropic.CacheControlEphemeralTypeEphemeral),
				}),
			},
		}),
		Messages:   anthropic.F(anthropicMessages),
		ToolChoice: anthropic.F(anthropic.ToolChoiceUnionParam(anthropic.ToolChoiceAutoParam{})),
		Tools:      anthropic.F(tools),
	})
	defer stream.Close()

	message := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		message.Accumulate(event)

		switch delta := event.Delta.(type) {
		case anthropic.ContentBlockDeltaEventDelta:
			if delta.Text != "" {
				print(delta.Text)
			}
		}
	}

	if stream.Err() != nil {
		return nil, fmt.Errorf("failed to stream response: %w", stream.Err())
	}


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
