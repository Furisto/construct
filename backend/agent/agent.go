package agent

import (
	"context"
	"fmt"

	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/tool"
	"github.com/google/uuid"
)

type AgentOptions struct {
	SystemPrompt   string
	ModelProviders []model.ModelProvider
	Toolbox        *tool.Toolbox
	SystemMemory   Memory
	UserMemory     Memory
}

func DefaultAgentOptions() *AgentOptions {
	return &AgentOptions{
		ModelProviders: []model.ModelProvider{},
		Toolbox:        tool.NewToolbox(),
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


func WithTools(tools ...tool.Tool) AgentOption {
	return func(o *AgentOptions) {
		
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
	Toolbox        *tool.Toolbox
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

func (a *Agent) Run(ctx context.Context) error {
	for _, mp := range a.ModelProviders {
		resp, err := mp.InvokeModel(ctx, uuid.MustParse("0195b4e2-45b6-76df-b208-f48b7b0d5f51"), ConstructSystemPrompt, []model.Message{
			{
				Source: model.MessageSourceUser,
				Content: []model.ContentBlock{
					&model.TextContentBlock{
						Text: "Hello, how are you? Please write at least 200 words and then read the file /etc/passwd",
					},
				},
			},
		}, model.WithStreamHandler(func(ctx context.Context, message *model.Message) {
			for _, block := range message.Content {
				switch block := block.(type) {
				case *model.TextContentBlock:
					fmt.Print(block.Text)
				}
			}
		}), model.WithTools(tool.FilesystemTools()))

		if err != nil {
			return err
		}

		fmt.Println(resp.Message.Content[0].(*model.TextContentBlock).Text)
		fmt.Println(resp.Usage)
	}
	return nil
}

func (a *Agent) NewTask() *Task {
	return &Task{}
}
