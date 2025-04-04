package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/furisto/construct/backend/api"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/agent"
	memory_model "github.com/furisto/construct/backend/memory/model"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/memory/task"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/tool"
	"github.com/google/uuid"
	"k8s.io/client-go/util/workqueue"
)

type RuntimeOptions struct {
	Tools       []tool.Tool
	Concurrency int
	ServerPort  int
}

func DefaultRuntimeOptions() *RuntimeOptions {
	return &RuntimeOptions{
		Tools:       []tool.Tool{},
		Concurrency: 5,
		ServerPort:  29333,
	}
}

type RuntimeOption func(*RuntimeOptions)

func WithTools(tools ...tool.Tool) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.Tools = tools
	}
}

func WithConcurrency(concurrency int) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.Concurrency = concurrency
	}
}

func WithServerPort(port int) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.ServerPort = port
	}
}

type Runtime struct {
	api        *api.Server
	memory     *memory.Client
	encryption *secret.Client
	toolbox    *tool.Toolbox

	concurrency int
	queue       workqueue.TypedDelayingInterface[uuid.UUID]
	running     atomic.Bool
}

func NewRuntime(memory *memory.Client, encryption *secret.Client, opts ...RuntimeOption) *Runtime {
	options := DefaultRuntimeOptions()
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

	runtime := &Runtime{
		memory:     memory,
		encryption: encryption,
		toolbox:    toolbox,

		concurrency: options.Concurrency,
		queue:       queue,
	}

	api := api.NewServer(runtime, options.ServerPort)
	runtime.api = api

	return runtime
}

func (a *Runtime) Run(ctx context.Context) error {
	if !a.running.CompareAndSwap(false, true) {
		return nil
	}

	go func() {
		err := a.api.ListenAndServe()
		if err != nil {
			slog.Error("failed to start api", "error", err)
		}
	}()

	var wg sync.WaitGroup
	for range a.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				taskID, shutdown := a.queue.Get()
				if shutdown {
					return
				}
				err := a.processTask(ctx, taskID)
				if err != nil {
					slog.Error("failed to process task", "error", err)
				}
			}
		}()
	}
	wg.Wait()
	return nil
}

func (a *Runtime) processTask(ctx context.Context, taskID uuid.UUID) error {
	defer a.queue.Done(taskID)

	task, err := a.memory.Task.Query().Where(task.IDEQ(taskID)).WithAgent().Only(ctx)
	if err != nil {
		return err
	}

	agent, err := a.memory.Agent.Query().Where(agent.IDEQ(task.AgentID)).Only(ctx)
	if err != nil {
		return err
	}

	m, err := a.memory.Model.Query().Where(memory_model.IDEQ(agent.DefaultModel)).WithModelProvider().Only(ctx)
	if err != nil {
		return err
	}

	providerAPI, err := a.modelProviderAPI(m)
	if err != nil {
		return err
	}

	resp, err := providerAPI.InvokeModel(ctx, m.Name, ConstructSystemPrompt, []model.Message{
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
	}), model.WithTools(a.toolbox.ListTools()...))

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

	a.memory.Message.Create().
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

func (a *Runtime) modelProviderAPI(m *memory.Model) (model.ModelProvider, error) {
	if m.Edges.ModelProvider == nil {
		return nil, fmt.Errorf("model provider not found")
	}
	provider := m.Edges.ModelProvider

	switch provider.ProviderType {
	case types.ModelProviderTypeAnthropic:
		secret, err := secret.GetSecret[model.AnthropicSecret](secret.ModelProviderSecret(provider.ID))
		if err != nil {
			return nil, err
		}

		provider, err := model.NewAnthropicProvider(secret.APIKey)
		if err != nil {
			return nil, err
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("unknown model provider type: %s", provider.ProviderType)
	}
}

func (a *Runtime) GetEncryption() *secret.Client {
	return a.encryption
}

func (a *Runtime) GetMemory() *memory.Client {
	return a.memory
}

func (a *Runtime) TriggerReconciliation(taskID uuid.UUID) {
	a.queue.Add(taskID)
}
