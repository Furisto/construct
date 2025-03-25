package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/furisto/construct/backend/memory"
	memory_model "github.com/furisto/construct/backend/memory/model"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/tool"
	"github.com/google/uuid"
	"k8s.io/client-go/util/workqueue"
)

type AgentOptions struct {
	AgentID        uuid.UUID
	SystemPrompt   string
	ModelProviders []model.ModelProvider
	Tools          []tool.Tool
	Mailbox        Memory
	Concurrency    int
	Memory         *memory.Client
}

func DefaultAgentOptions() *AgentOptions {
	return &AgentOptions{
		AgentID:        uuid.New(),
		ModelProviders: []model.ModelProvider{},
		SystemPrompt:   "You are a helpful assistant that can help with tasks and answer questions.",
		Tools:          []tool.Tool{},
		Mailbox:        NewEphemeralMemory(),
		Concurrency:    5,
	}
}

type AgentOption func(*AgentOptions)

func WithSystemPrompt(systemPrompt string) AgentOption {
	return func(o *AgentOptions) {
		o.SystemPrompt = systemPrompt
	}
}

func WithModelProviders(modelProviders ...model.ModelProvider) AgentOption {
	return func(o *AgentOptions) {
		o.ModelProviders = modelProviders
	}
}

func WithTools(tools ...tool.Tool) AgentOption {
	return func(o *AgentOptions) {
		o.Tools = tools
	}
}

func WithMailbox(mailbox Memory) AgentOption {
	return func(o *AgentOptions) {
		o.Mailbox = mailbox
	}
}

func WithMemory(memory *memory.Client) AgentOption {
	return func(o *AgentOptions) {
		o.Memory = memory
	}
}

func WithConcurrency(concurrency int) AgentOption {
	return func(o *AgentOptions) {
		o.Concurrency = concurrency
	}
}

func WithAgentID(agentID uuid.UUID) AgentOption {
	return func(o *AgentOptions) {
		o.AgentID = agentID
	}
}

type Agent struct {
	AgentID        uuid.UUID
	ModelProviders []model.ModelProvider
	SystemPrompt   string
	Toolbox        *tool.Toolbox
	Mailbox        *Mailbox
	Memory         *memory.Client
	Concurrency    int
	Queue          workqueue.TypedDelayingInterface[uuid.UUID]
	running        atomic.Bool
}

func NewAgent(opts ...AgentOption) *Agent {
	options := DefaultAgentOptions()
	for _, opt := range opts {
		opt(options)
	}
	toolbox := tool.NewToolbox()
	for _, tool := range options.Tools {
		toolbox.AddTool(tool)
	}

	queue := workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[uuid.UUID]{
		Name: "construct",
	})

	return &Agent{
		AgentID:        options.AgentID,
		ModelProviders: options.ModelProviders,
		SystemPrompt:   options.SystemPrompt,
		Toolbox:        toolbox,
		Mailbox:        NewMailbox(),
		Memory:         options.Memory,
		Concurrency:    options.Concurrency,
		Queue:          queue,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	if !a.running.CompareAndSwap(false, true) {
		return nil
	}

	var wg sync.WaitGroup
	for range a.Concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				taskID, shutdown := a.Queue.Get()
				if shutdown {
					return
				}
				a.processTask(ctx, taskID)
			}
		}()
	}
	wg.Wait()
	return nil
}

func (a *Agent) processTask(ctx context.Context, taskID uuid.UUID) error {
	defer a.Queue.Done(taskID)

	task, err := a.Memory.Task.Get(ctx, taskID)
	if err != nil {
		return err
	}

	userMessages := a.Mailbox.Dequeue(taskID)
	for _, message := range userMessages {
		a.Memory.Message.Create().
			SetRole(types.MessageRoleUser).
			SetContent(message).
			Save(ctx)
	}

	m, err := a.Memory.Model.Query().Where(memory_model.ID(task.Spec.ModelID)).WithModelProvider().Only(ctx)
	if err != nil {
		return err
	}

	modelProvider, err := a.modelProviderForModel(m)
	if err != nil {
		return err
	}

	resp, err := modelProvider.InvokeModel(ctx, uuid.MustParse("0195b4e2-45b6-76df-b208-f48b7b0d5f51"), ConstructSystemPrompt, []model.Message{
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

	for _, block := range resp.Message.Content {
		switch block := block.(type) {
		case *model.TextContentBlock:
			fmt.Print(block.Text)
		case *model.ToolCallContentBlock:
			fmt.Println(block.Name)
			fmt.Println(string(block.Input))
		}
	}

	a.Memory.Message.Create().
		SetRole(types.MessageRoleAssistant).
		SetUsage(&types.MessageUsage{
			InputTokens:      resp.Usage.InputTokens,
			OutputTokens:     resp.Usage.OutputTokens,
			CacheWriteTokens: resp.Usage.CacheWriteTokens,
			CacheReadTokens:  resp.Usage.CacheReadTokens,
		}).
		Save(ctx)

	return nil
}

func (a *Agent) modelProviderForModel(m *memory.Model) (model.ModelProvider, error) {
	if m.Edges.ModelProvider == nil {
		return nil, fmt.Errorf("model provider not found")
	}

	switch m.Edges.ModelProvider.ProviderType {
	case types.ModelProviderTypeAnthropic:
		secret, err := secret.GetSecret[model.AnthropicSecret](secret.ModelProviderSecret(m.Edges.ModelProvider.ID))
		if err != nil {
			return nil, err
		}

		provider, err := model.NewAnthropicProvider(secret.APIKey)
		if err != nil {
			return nil, err
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("unknown model provider type: %s", m.Edges.ModelProvider.ProviderType)
	}
}

func (a *Agent) SendMessage(taskID uuid.UUID, message string) {
	a.Mailbox.Enqueue(taskID, message)
	a.Queue.Add(taskID)
}

func (a *Agent) CreateTask(ctx context.Context) (uuid.UUID, error) {
	task, err := a.Memory.Task.Create().
		SetAgentID(a.AgentID).
		SetInputTokens(0).
		SetOutputTokens(0).
		SetCacheWriteTokens(0).
		SetCacheReadTokens(0).
		Save(ctx)

	if err != nil {
		return uuid.Nil, err
	}

	return task.ID, nil
}
