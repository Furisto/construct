package agent

import (
	"context"

	"github.com/furisto/construct/backend/model"
)

type Agent struct {
	ModelProvider []model.ModelProvider
}

func NewAgent(modelProviders []model.ModelProvider) *Agent {
	return &Agent{
		ModelProvider: modelProviders,
	}
}

func (a *Agent) Run(ctx context.Context) {

}



