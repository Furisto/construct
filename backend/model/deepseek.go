package model

import (
	"context"

	"github.com/cohesion-org/deepseek-go"
)

type DeepSeekProvider struct {
	client *deepseek.Client
}

func NewDeepSeekProvider(apiKey string) (*DeepSeekProvider, error) {
	return nil, nil
}

func (p *DeepSeekProvider) ListModels(ctx context.Context) ([]Model, error) {
	return nil, nil
}
