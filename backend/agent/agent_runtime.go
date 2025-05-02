package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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

	interpreter := NewCodeInterpreter(options.Tools,
		NewInputOutputInterceptor(),
		InterpreterInterceptor(ToolNameInterceptor),
	)

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

	task, agent, err := a.fetchTaskWithAgent(ctx, taskID)
	if err != nil {
		return err
	}

	processedMessages, nextMessage, err := a.fetchTaskMessages(task.ID)
	if err != nil {
		return err
	}

	if nextMessage == nil {
		slog.DebugContext(ctx, "no unprocessed messages, skipping", "task_id", taskID)
		return nil
	}

	modelProvider, err := a.createModelProvider(ctx, agent)
	if err != nil {
		return err
	}

	modelMessages, err := a.prepareModelData(processedMessages, nextMessage)
	if err != nil {
		return err
	}

	systemPrompt := a.appendToolInstruction(agent.Instructions)
	resp, err := a.invokeModel(ctx, modelProvider, agent.Edges.Model.Name, systemPrompt, modelMessages)
	if err != nil {
		return err
	}

	newMessage, err := a.saveResponse(ctx, taskID, nextMessage, resp, agent.Edges.Model)
	if err != nil {
		return err
	}

	err = a.callTools(ctx, taskID, resp.Message.Content)
	if err != nil {
		return err
	}

	a.messageHub.Publish(taskID, newMessage)
	a.TriggerReconciliation(taskID)

	return nil
}

func (a *Runtime) fetchTaskWithAgent(ctx context.Context, taskID uuid.UUID) (*memory.Task, *memory.Agent, error) {
	task, err := a.memory.Task.Query().Where(memory_task.IDEQ(taskID)).WithAgent().Only(ctx)
	if err != nil {
		return nil, nil, err
	}

	if task.AgentID == uuid.Nil {
		return nil, nil, fmt.Errorf("task has no agent: %s", taskID)
	}

	return task, task.Edges.Agent, nil
}

func (a *Runtime) fetchTaskMessages(taskID uuid.UUID) ([]*memory.Message, *memory.Message, error) {
	messages, err := a.memory.Message.Query().
		Where(memory_message.TaskIDEQ(taskID)).
		Order(memory_message.ByCreateTime()).
		All(context.Background())
	if err != nil {
		return nil, nil, err
	}

	categorized := map[string][]*memory.Message{
		"processed":            make([]*memory.Message, 0),
		"unprocessedUser":      make([]*memory.Message, 0),
		"unprocessedAssistant": make([]*memory.Message, 0),
		"unprocessedSystem":    make([]*memory.Message, 0),
	}

	for _, message := range messages {
		if message.ProcessedTime.IsZero() {
			switch message.Source {
			case types.MessageSourceUser:
				categorized["unprocessedUser"] = append(categorized["unprocessedUser"], message)
			case types.MessageSourceAssistant:
				categorized["unprocessedAssistant"] = append(categorized["unprocessedAssistant"], message)
			case types.MessageSourceSystem:
				categorized["unprocessedSystem"] = append(categorized["unprocessedSystem"], message)
			}
		} else {
			categorized["processed"] = append(categorized["processed"], message)
		}
	}

	var nextMessage *memory.Message
	switch {
	case len(categorized["unprocessedSystem"]) > 0:
		nextMessage = categorized["unprocessedSystem"][0]
	case len(categorized["unprocessedAssistant"]) > 0:
		nextMessage = categorized["unprocessedAssistant"][0]
	case len(categorized["unprocessedUser"]) > 0:
		nextMessage = categorized["unprocessedUser"][0]
	}

	return categorized["processed"], nextMessage, nil
}

func (a *Runtime) createModelProvider(ctx context.Context, agent *memory.Agent) (model.ModelProvider, error) {
	m, err := a.memory.Model.Query().Where(memory_model.IDEQ(agent.DefaultModel)).WithModelProvider().Only(ctx)
	if err != nil {
		return nil, err
	}

	providerAPI, err := a.modelProviderAPI(m)
	if err != nil {
		return nil, err
	}

	return providerAPI, nil
}

func (a *Runtime) prepareModelData(
	processedMessages []*memory.Message,
	nextMessage *memory.Message,
) ([]*model.Message, error) {
	modelMessages := make([]*model.Message, 0, len(processedMessages))
	for _, msg := range processedMessages {
		modelMsg, err := conv.ConvertMemoryMessageToModel(msg)
		if err != nil {
			return nil, err
		}
		modelMessages = append(modelMessages, modelMsg)
	}

	modelMsg, err := conv.ConvertMemoryMessageToModel(nextMessage)
	if err != nil {
		return nil, err
	}
	modelMessages = append(modelMessages, modelMsg)

	return modelMessages, nil
}

func (a *Runtime) appendToolInstruction(agentInstruction string) string {
	var toolInstruction strings.Builder
	toolInstruction.WriteString(`
# Tool Instructions
You can use the following tools to help you answer the user's question. The tools are specified as Javascript functions.
In order to use them you have to write a javascript program and then call the code interpreter tool with the script as argument.
The only functions that are allowed for this javascript program are the ones specified in the tool descriptions.
The script will be executed in a new process, so you don't need to worry about the environment it is executed in.
If you try to call any other function that is not specified here the execution will fail.
`)

	toolInstruction.WriteString("\n\n")
	for _, tool := range a.interpreter.Tools {
		toolInstruction.WriteString(fmt.Sprintf("# %s\n%s\n\n", tool.Name(), tool.Description()))
	}

	return fmt.Sprintf("%s\n\n%s", agentInstruction, toolInstruction.String())
}

func (a *Runtime) invokeModel(ctx context.Context, providerAPI model.ModelProvider, modelName, instructions string, modelMessages []*model.Message) (*model.ModelResponse, error) {
	return providerAPI.InvokeModel(
		ctx,
		modelName,
		instructions,
		modelMessages,
		model.WithStreamHandler(func(ctx context.Context, message *model.Message) {
			for _, block := range message.Content {
				switch block := block.(type) {
				case *model.TextBlock:
					fmt.Print(block.Text)
				case *model.ToolCallBlock:
					fmt.Println(block.Args)
				}
			}
		}),
		model.WithTools(a.interpreter),
	)
}

func (a *Runtime) saveResponse(ctx context.Context, taskID uuid.UUID, messageToProcess *memory.Message, resp *model.ModelResponse, m *memory.Model) (*memory.Message, error) {
	_, err := messageToProcess.Update().SetProcessedTime(time.Now()).Save(ctx)
	if err != nil {
		return nil, err
	}

	memoryContent, err := conv.ConvertModelContentBlocksToMemory(resp.Message.Content)
	if err != nil {
		return nil, err
	}

	cost := calculateCost(resp.Usage, m)

	t := time.Now()
	newMessage, err := a.memory.Message.Create().
		SetTaskID(taskID).
		SetSource(types.MessageSourceAssistant).
		SetContent(memoryContent).
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
		return nil, err
	}

	_, err = a.memory.Task.UpdateOneID(taskID).
		AddInputTokens(resp.Usage.InputTokens).
		AddOutputTokens(resp.Usage.OutputTokens).
		AddCacheWriteTokens(resp.Usage.CacheWriteTokens).
		AddCacheReadTokens(resp.Usage.CacheReadTokens).
		AddCost(cost).
		Save(ctx)

	if err != nil {
		return nil, err
	}

	return newMessage, nil
}

func (a *Runtime) callTools(ctx context.Context, taskID uuid.UUID, content []model.ContentBlock) error {
	var toolResults []string

	for _, block := range content {
		toolCall, ok := block.(*model.ToolCallBlock)
		if !ok {
			continue
		}

		fsys := afero.NewBasePathFs(afero.NewOsFs(), "/tmp")
		result, err := a.interpreter.Run(ctx, fsys, toolCall.Args)
		if err != nil {
			return err
		}

		toolResults = append(toolResults, result)
	}

	if len(toolResults) > 0 {
		toolBlocks := make([]types.MessageBlock, 0, len(toolResults))
		for _, result := range toolResults {
			toolBlocks = append(toolBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindCodeActToolResult,
				Payload: result,
			})
		}

		_, err := a.memory.Message.Create().
			SetTaskID(taskID).
			SetSource(types.MessageSourceSystem).
			SetContent(&types.MessageContent{
				Blocks: toolBlocks,
			}).
			Save(ctx)

		if err != nil {
			return err
		}
	}

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

func calculateCost(usage model.Usage, model *memory.Model) float64 {
	return float64(usage.InputTokens)*model.InputCost +
		float64(usage.OutputTokens)*model.OutputCost +
		float64(usage.CacheWriteTokens)*model.CacheWriteCost +
		float64(usage.CacheReadTokens)*model.CacheReadCost
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
