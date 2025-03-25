package model

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type OpenAIProvider struct {
	client *openai.Client
}

func NewOpenAIProvider(apiKey string) (*OpenAIProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openai API key is required")
	}

	return &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(apiKey)),
	}, nil
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]Model, error) {
	resp, err := p.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list openai models: %w", err)
	}

	var models []Model
	for _, model := range resp.Data {
		models = append(models, Model{
			Name:     model.ID,
			Provider: "openai",
		})
	}

	return models, nil
}
