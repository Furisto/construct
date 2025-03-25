package agent

import (
	"context"

	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/toolbox"
)

type AgentOptions struct {
	SystemPrompt   string
	ModelProviders []model.ModelProvider
	Toolbox        *toolbox.Toolbox
	SystemMemory   Memory
	UserMemory     Memory
}

func DefaultAgentOptions() *AgentOptions {
	return &AgentOptions{
		ModelProviders: []model.ModelProvider{},
		Toolbox:        toolbox.NewToolbox(),
	}
}

type AgentOption func(*AgentOptions)

func WithSystemPrompt(systemPrompt string) AgentOption {
	return func(o *AgentOptions) {
		o.SystemPrompt = systemPrompt
	}
}

func WithModelProviders(modelProviders []model.ModelProvider) AgentOption {
	return func(o *AgentOptions) {
		o.ModelProviders = modelProviders
	}
}

func WithToolbox(toolbox *toolbox.Toolbox) AgentOption {
	return func(o *AgentOptions) {
		o.Toolbox = toolbox
	}
}

func WithSystemMemory(memory Memory) AgentOption {
	return func(o *AgentOptions) {
		o.SystemMemory = memory
	}
}

func WithUserMemory(memory Memory) AgentOption {
	return func(o *AgentOptions) {
		o.UserMemory = memory
	}
}

type Agent struct {
	ModelProviders []model.ModelProvider
	Toolbox        *toolbox.Toolbox
	SystemPrompt   string
}

func NewAgent(opts ...AgentOption) *Agent {
	options := DefaultAgentOptions()
	for _, opt := range opts {
		opt(options)
	}
	return &Agent{
		ModelProviders: options.ModelProviders,
		Toolbox:        options.Toolbox,
	}
}

func (a *Agent) Run(ctx context.Context) {

}
