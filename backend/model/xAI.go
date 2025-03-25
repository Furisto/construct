package model

import (
	"context"
)

type XAIProvider struct {
}

func NewXAIProvider(apiKey string) (*XAIProvider, error) {
	return nil, nil
}

func (p *XAIProvider) ListModels(ctx context.Context) ([]Model, error) {
	return nil, nil
}
