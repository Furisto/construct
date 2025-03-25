package modelprovider

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicProvider struct {
	client *anthropic.Client
}

func NewAnthropicProvider(apiKey string) (*AnthropicProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic API key is required")
	}

	return &AnthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}, nil
}

func (p *AnthropicProvider) InvokeModel(ctx context.Context, model, systemPrompt string, opts ...InvokeModelOption) (*ModelResponse, error) {
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	if systemPrompt == "" {
		return nil, fmt.Errorf("system prompt is required")
	}

	o := &InvokeModelOptions{}
	for _, opt := range opts {
		opt(o)
	}

	var messages []anthropic.MessageParam
	if len(o.Messages) == 0 {
		messages = []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(systemPrompt)),
		}
	} else {
		messages = make([]anthropic.MessageParam, len(o.Messages))
		for i, message := range o.Messages {
			messages[i] = anthropic.NewUserMessage(anthropic.NewTextBlock(message.Content))
		}
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(systemPrompt)))
	}

	resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(model),
		MaxTokens: anthropic.F(int64(o.MaxTokens)),
		Temperature: anthropic.F(o.Temperature),
		System:    anthropic.F([]anthropic.TextBlockParam{anthropic.NewTextBlock(systemPrompt)}),
		Messages:  anthropic.F(messages),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to invoke anthropic provider: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response from Anthropic API")
	}

	var modelResponse []ContentBlock
	for _, block := range resp.Content {
		switch block := block.AsUnion().(type) {
		case anthropic.TextBlock:
			println(block.Text)
		case anthropic.ToolUseBlock:
			println(block.Name + ": " + string(block.Input))
		}
	}

	return &ModelResponse {
		Blocks: modelResponse,
		Usage: Usage{
			InputTokens: resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			CacheWriteTokens: resp.Usage.CacheCreationInputTokens,
			CacheReadTokens: resp.Usage.CacheReadInputTokens,
		},
	}, nil
}

func (p *AnthropicProvider) ListModels(ctx context.Context) ([]Model, error) {
	resp, err := p.client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list anthropic models: %w", err)
	}

	var models []Model
	for _, model := range resp.Data {
		models = append(models, Model{
			Name:     model.ID,
			Provider: "anthropic",
		})
	}

	return models, nil
}
