package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/furisto/construct/backend/agent/conv"
	"github.com/furisto/construct/backend/api"
	"github.com/furisto/construct/backend/memory"
	memory_message "github.com/furisto/construct/backend/memory/message"
	memory_model "github.com/furisto/construct/backend/memory/model"
	"github.com/furisto/construct/backend/memory/schema/types"
	memory_task "github.com/furisto/construct/backend/memory/task"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/stream"
	"github.com/furisto/construct/backend/tool"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"k8s.io/client-go/util/workqueue"
)

const DefaultServerPort = 29333

type RuntimeOptions struct {
	Tools       []tool.CodeActTool
	Concurrency int
	ServerPort  int
}

func DefaultRuntimeOptions() *RuntimeOptions {
	return &RuntimeOptions{
		Tools:       []tool.CodeActTool{},
		Concurrency: 5,
		ServerPort:  DefaultServerPort,
	}
}

type RuntimeOption func(*RuntimeOptions)

func WithCodeActTools(tools ...tool.CodeActTool) RuntimeOption {
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
	api         *api.Server
	memory      *memory.Client
	encryption  *secret.Client
	messageHub  *stream.EventHub
	concurrency int
	queue       workqueue.TypedDelayingInterface[uuid.UUID]
	interpreter *CodeInterpreter
	running     atomic.Bool
}

func NewRuntime(memory *memory.Client, encryption *secret.Client, opts ...RuntimeOption) (*Runtime, error) {
	options := DefaultRuntimeOptions()
	for _, opt := range opts {
		opt(options)
	}

	queue := workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[uuid.UUID]{
		Name: "construct",
	})

	messageHub, err := stream.NewMessageHub(memory)
	if err != nil {
		return nil, err
	}

	interpreter := NewCodeInterpreter(options.Tools)

	runtime := &Runtime{
		memory:     memory,
		encryption: encryption,

		messageHub:  messageHub,
		concurrency: options.Concurrency,
		queue:       queue,
		interpreter: interpreter,
	}

	api := api.NewServer(runtime, options.ServerPort)
	runtime.api = api

	return runtime, nil
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

	task, err := a.memory.Task.Query().Where(memory_task.IDEQ(taskID)).WithAgent().Only(ctx)
	if err != nil {
		return err
	}

	if task.AgentID == uuid.Nil {
		slog.Info("task has no agent, skipping", "task_id", taskID)
		return nil
	}

	messages, err := a.memory.Message.Query().
		Where(memory_message.TaskIDEQ(taskID)).
		Order(memory_message.ByCreateTime()).
		All(ctx)
	if err != nil {
		return err
	}

	processedMessages := make([]*memory.Message, 0)
	unprocessedUserMessages := make([]*memory.Message, 0)
	unprocessedAssistantMessages := make([]*memory.Message, 0)

	for _, message := range messages {
		if message.ProcessedTime.IsZero() {
			if message.Source == types.MessageSourceUser {
				unprocessedUserMessages = append(unprocessedUserMessages, message)
			} else if message.Source == types.MessageSourceAssistant {
				unprocessedAssistantMessages = append(unprocessedAssistantMessages, message)
			}
		} else {
			processedMessages = append(processedMessages, message)
		}
	}

	if len(unprocessedUserMessages) == 0 && len(unprocessedAssistantMessages) == 0 {
		slog.Info("no unprocessed messages, skipping", "task_id", taskID)
		return nil
	}

	agent := task.Edges.Agent
	m, err := a.memory.Model.Query().Where(memory_model.IDEQ(agent.DefaultModel)).WithModelProvider().Only(ctx)
	if err != nil {
		return err
	}

	providerAPI, err := a.modelProviderAPI(m)
	if err != nil {
		return err
	}

	var messageToProcess *memory.Message
	if len(unprocessedAssistantMessages) > 0 {
		messageToProcess = unprocessedAssistantMessages[0]
	} else if len(unprocessedUserMessages) > 0 {
		messageToProcess = unprocessedUserMessages[0]
	}

	modelMessages := make([]model.Message, 0, len(processedMessages))
	for _, msg := range processedMessages {
		modelMsg, err := conv.ConvertMemoryMessageToModel(msg)
		if err != nil {
			return err
		}
		modelMessages = append(modelMessages, modelMsg)
	}

	modelMsg, err := conv.ConvertMemoryMessageToModel(messageToProcess)
	if err != nil {
		return err
	}
	modelMessages = append(modelMessages, modelMsg)

	var toolDescriptions strings.Builder
	toolDescriptions.WriteString("You can use the following tools to help you answer the user's question. The tools are specified as Javascript functions." +
		"In order to use them you have to write a javascript program and then call the code interpreter tool with the script as argument." +
		"The only functions that are allowed for this javascript program are the ones specified in the tool descriptions." +
		"The script will be executed in a new process, so you don't need to worry about the environment it is executed in." +
		"If you try to call any other function that is not specified here the execution will fail." +
		"\n\n")
	for _, tool := range a.interpreter.Tools {
		toolDescriptions.WriteString(fmt.Sprintf("%s: %s\n", tool.Name(), tool.Description()))
	}

	instructions := fmt.Sprintf("%s\n\n%s", agent.Instructions, toolDescriptions.String())
	os.WriteFile("/tmp/tool_descriptions.txt", []byte(instructions), 0644)

	resp, err := providerAPI.InvokeModel(
		ctx,
		m.Name,
		instructions,
		modelMessages,
		model.WithStreamHandler(func(ctx context.Context, message *model.Message) {
			for _, block := range message.Content {
				switch block := block.(type) {
				case *model.TextBlock:
					fmt.Print(block.Text)
				}
			}
		}),
		model.WithTools(a.interpreter),
	)

	if err != nil {
		return err
	}

	_, err = messageToProcess.Update().SetProcessedTime(time.Now()).Save(ctx)
	if err != nil {
		return err
	}

	cost := float64(resp.Usage.InputTokens)*m.InputCost +
		float64(resp.Usage.OutputTokens)*m.OutputCost +
		float64(resp.Usage.CacheWriteTokens)*m.CacheWriteCost +
		float64(resp.Usage.CacheReadTokens)*m.CacheReadCost

	fmt.Printf("%+v\n", resp.Message.Content)

	messageContent, err := conv.ConvertModelMessageToMemory(resp.Message)
	if err != nil {
		return err
	}
	t := time.Now()
	newMessage, err := a.memory.Message.Create().
		SetTaskID(taskID).
		SetSource(types.MessageSourceAssistant).
		SetContent(messageContent).
		SetCreateTime(t).
		SetProcessedTime(t).
		SetUsage(&types.MessageUsage{
			InputTokens:      resp.Usage.InputTokens,
			OutputTokens:     resp.Usage.OutputTokens,
			CacheWriteTokens: resp.Usage.CacheWriteTokens,
			CacheReadTokens:  resp.Usage.CacheReadTokens,
			Cost:             cost,
		}).
		Save(ctx)

	if err != nil {
		return err
	}

	_, err = task.Update().
		AddInputTokens(resp.Usage.InputTokens).
		AddOutputTokens(resp.Usage.OutputTokens).
		AddCacheWriteTokens(resp.Usage.CacheWriteTokens).
		AddCacheReadTokens(resp.Usage.CacheReadTokens).
		AddCost(cost).
		Save(ctx)

	if err != nil {
		return err
	}

	for _, block := range resp.Message.Content {
		switch block := block.(type) {
		case *model.ToolCallBlock:
			fsys := afero.NewBasePathFs(afero.NewOsFs(), "/tmp")
			result, err := a.interpreter.Run(ctx, fsys, block.Args)
			if err != nil {
				return err
			}

			fmt.Println(result)
		}
	}
	if err != nil {
		return err
	}

	a.messageHub.Publish(taskID, newMessage)

	a.TriggerReconciliation(taskID)

	return nil
}

func (a *Runtime) modelProviderAPI(m *memory.Model) (model.ModelProvider, error) {
	if m.Edges.ModelProvider == nil {
		return nil, fmt.Errorf("model provider not found")
	}
	provider := m.Edges.ModelProvider

	switch provider.ProviderType {
	case types.ModelProviderTypeAnthropic:
		providerAuth, err := a.encryption.Decrypt(provider.Secret, []byte(secret.ModelProviderSecret(provider.ID)))
		if err != nil {
			return nil, err
		}

		slog.Info("provider auth", "auth", string(providerAuth))

		var auth struct {
			APIKey string `json:"apiKey"`
		}
		err = json.Unmarshal(providerAuth, &auth)
		if err != nil {
			return nil, err
		}

		provider, err := model.NewAnthropicProvider(auth.APIKey)
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

func (a *Runtime) GetMessageHub() *stream.EventHub {
	return a.messageHub
}
