package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/furisto/construct/backend/api"
	"github.com/furisto/construct/backend/memory"
	memory_message "github.com/furisto/construct/backend/memory/message"
	memory_model "github.com/furisto/construct/backend/memory/model"
	"github.com/furisto/construct/backend/memory/schema/types"
	memory_task "github.com/furisto/construct/backend/memory/task"
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

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := a.api.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			slog.Error("API server failed", "error", err)
		}
	}()

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

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	err := a.api.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error("failed to shutdown API server", "error", err)
	}

	a.queue.ShutDownWithDrain()

	stop := make(chan struct{})
	go func() {
		wg.Wait()
		close(stop)
	}()

	select {
	case <-stop:
		return nil
	case <-shutdownCtx.Done():
		return shutdownCtx.Err()
	}
}

func (a *Runtime) processTask(ctx context.Context, taskID uuid.UUID) error {
	defer a.queue.Done(taskID)

	// check if agent is assigned to task
	task, err := a.memory.Task.Query().Where(memory_task.IDEQ(taskID)).WithAgent().Only(ctx)
	if err != nil {
		return err
	}

	if task.Edges.Agent == nil {
		slog.Info("task has no agent, skipping", "task_id", taskID)
		return nil
	}

	// check if there are unprocessed messages
	messages, err := a.memory.Message.Query().Where(memory_message.TaskIDEQ(taskID)).All(ctx)
	if err != nil {
		return err
	}

	unprocessedMessages := make([]*memory.Message, 0)
	for _, message := range messages {
		if message.ProcessedTime.IsZero() {
			unprocessedMessages = append(unprocessedMessages, message)
		}
	}

	if len(unprocessedMessages) == 0 {
		slog.Info("no unprocessed messages, skipping", "task_id", taskID)
		return nil
	}

	// figure out which model to use
	agent := task.Edges.Agent
	m, err := a.memory.Model.Query().Where(memory_model.IDEQ(agent.DefaultModel)).WithModelProvider().Only(ctx)
	if err != nil {
		return err
	}

	providerAPI, err := a.modelProviderAPI(m)
	if err != nil {
		return err
	}

	// invoke model
	resp, err := providerAPI.InvokeModel(ctx, m.Name, agent.Instructions, []model.Message{
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
