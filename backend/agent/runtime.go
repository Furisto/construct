package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/analytics"
	"github.com/furisto/construct/backend/api"
	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	memory_message "github.com/furisto/construct/backend/memory/message"
	memory_model "github.com/furisto/construct/backend/memory/model"
	"github.com/furisto/construct/backend/memory/schema/types"
	memory_task "github.com/furisto/construct/backend/memory/task"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/prompt"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/furisto/construct/backend/tool/native"
	"github.com/furisto/construct/shared/conv"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const DefaultServerPort = 29333

type RuntimeOptions struct {
	Tools       []codeact.Tool
	Concurrency int
	Analytics   analytics.Client
}

func DefaultRuntimeOptions() *RuntimeOptions {
	return &RuntimeOptions{
		Tools:       []codeact.Tool{},
		Concurrency: 5,
	}
}

type RuntimeOption func(*RuntimeOptions)

func WithCodeActTools(tools ...codeact.Tool) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.Tools = tools
	}
}

func WithConcurrency(concurrency int) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.Concurrency = concurrency
	}
}

func WithAnalytics(analytics analytics.Client) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.Analytics = analytics
	}
}

type Result struct {
	RetryAfter time.Duration
	Retry      bool
}

type Runtime struct {
	api        *api.Server
	memory     *memory.Client
	encryption *secret.Client
	eventHub   *event.MessageHub
	bus        *event.Bus

	concurrency int
	queue       workqueue.TypedDelayingInterface[uuid.UUID]
	running     atomic.Bool
	wg          sync.WaitGroup

	interpreter  *codeact.Interpreter
	runningTasks *SyncMap[uuid.UUID, context.CancelFunc]

	analytics analytics.Client
	metrics   *prometheus.Registry
}

func NewRuntime(memory *memory.Client, encryption *secret.Client, listener net.Listener, opts ...RuntimeOption) (*Runtime, error) {
	options := DefaultRuntimeOptions()
	for _, opt := range opts {
		opt(options)
	}

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsRegistry.MustRegister(collectors.NewGoCollector())
	metricsRegistry.MustRegister(collectors.NewBuildInfoCollector())
	metricsRegistry.MustRegister(collectors.NewDBStatsCollector(memory.MustDB(), "construct"))

	// Register workqueue metrics provider with custom registry
	wqProvider := newWorkqueueMetricsProvider(metricsRegistry)
	workqueue.SetProvider(wqProvider)

	queue := workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[uuid.UUID]{
		Name: "construct",
	})

	messageHub, err := event.NewMessageHub(memory)
	if err != nil {
		return nil, err
	}

	interceptors := []codeact.Interceptor{
		codeact.InterceptorFunc(codeact.ToolStatisticsInterceptor),
		codeact.InterceptorFunc(codeact.DurableFunctionInterceptor),
		codeact.NewToolEventPublisher(messageHub),
		codeact.InterceptorFunc(codeact.ResetTemporarySessionValuesInterceptor),
	}

	runtime := &Runtime{
		memory:       memory,
		encryption:   encryption,
		eventHub:     messageHub,
		bus:          event.NewBus(metricsRegistry),
		concurrency:  options.Concurrency,
		queue:        queue,
		interpreter:  codeact.NewInterpreter(options.Tools, interceptors),
		runningTasks: NewSyncMap[uuid.UUID, context.CancelFunc](),
		analytics:    options.Analytics,
		metrics:      metricsRegistry,
	}

	api := api.NewServer(runtime, listener, runtime.bus, runtime.analytics)
	runtime.api = api

	return runtime, nil
}

func (rt *Runtime) Run(ctx context.Context) error {
	if !rt.running.CompareAndSwap(false, true) {
		return nil
	}

	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		err := rt.api.ListenAndServe(ctx)
		if err != nil && err != http.ErrServerClosed {
			slog.Error("API server failed", "error", err)
		}
	}()

	for range rt.concurrency {
		rt.wg.Add(1)
		go rt.worker(ctx)
	}

	sub := event.Subscribe(rt.bus, func(ctx context.Context, e event.TaskEvent) {
		rt.queue.Add(e.TaskID)
	}, nil)

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	err := rt.api.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error("failed to shutdown API server", "error", err)
	}

	sub.Unsubscribe()
	rt.queue.ShutDownWithDrain()

	stop := make(chan struct{})
	go func() {
		rt.wg.Wait()
		close(stop)
	}()

	select {
	case <-stop:
		return nil
	case <-shutdownCtx.Done():
		return shutdownCtx.Err()
	}
}

func (rt *Runtime) publishError(err error, taskID uuid.UUID) {
	if err != nil {
		slog.Error("failed to process task", "error", err, "task_id", taskID)
	}

	if errors.Is(err, context.Canceled) {
		return
	}

	msg := NewSystemMessage(taskID, WithContent(&v1.MessagePart{
		Data: &v1.MessagePart_Error_{Error: &v1.MessagePart_Error{Message: err.Error()}},
	}))

	rt.eventHub.Publish(taskID, &v1.SubscribeResponse{
		Event: &v1.SubscribeResponse_Message{
			Message: msg,
		},
	})
}

func (rt *Runtime) worker(ctx context.Context) {
	defer rt.wg.Done()

	for {
		taskID, shutdown := rt.queue.Get()
		if shutdown {
			return
		}

		result, err := rt.processTask(ctx, taskID)
		if err != nil {
			slog.ErrorContext(ctx, "task could not be processed", "error", err, "task_id", taskID)
			rt.publishError(err, taskID)
		}

		switch {
		case result.RetryAfter > 0:
			rt.queue.AddAfter(taskID, result.RetryAfter)
		case result.Retry:
			rt.queue.Add(taskID)
		}

		rt.queue.Done(taskID)
	}
}

func (rt *Runtime) processTask(ctx context.Context, taskID uuid.UUID) (Result, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "panic in processTask", "error", r)
		}
	}()
	slog.DebugContext(ctx, "processing task", "task_id", taskID)

	ctx, cancel := context.WithCancel(ctx)
	rt.runningTasks.Set(taskID, cancel)
	defer rt.runningTasks.Delete(taskID)
	defer cancel()

	task, agent, err := rt.fetchTaskWithAgent(ctx, taskID)
	if err != nil {
		return Result{}, err
	}
	
	processedMessages, nextMessage, err := rt.fetchTaskMessages(ctx, task.ID)
	if err != nil {
		return Result{}, err
	}

	if nextMessage == nil {
		slog.DebugContext(ctx, "no unprocessed messages, skipping", "task_id", taskID)
		return Result{}, nil
	}
	rt.setTaskPhaseAndPublish(ctx, taskID, types.TaskPhaseRunning)
	defer rt.setTaskPhaseAndPublish(ctx, taskID, types.TaskPhaseAwaiting)

	if nextMessage.Source == types.MessageSourceUser {
		msg, err := ConvertMemoryMessageToProto(nextMessage)
		if err != nil {
			return Result{}, err
		}
		rt.publishMessage(taskID, msg)
	}

	modelProvider, err := rt.createModelProviderClient(ctx, agent)
	if err != nil {
		return Result{}, err
	}

	modelMessages, err := rt.prepareModelData(processedMessages, nextMessage)
	if err != nil {
		return Result{}, err
	}

	systemPrompt, err := rt.assembleSystemPrompt(ctx, agent.Instructions, task.ProjectDirectory)
	if err != nil {
		return Result{}, err
	}

	slog.DebugContext(ctx, "invoking model", "task_id", taskID, "model", agent.Edges.Model.Name, "agent_id", agent.ID)
	message, err := modelProvider.InvokeModel(
		ctx,
		agent.Edges.Model.Name,
		systemPrompt,
		modelMessages,
		model.WithStreamHandler(func(ctx context.Context, chunk string) {
			rt.publishMessage(taskID, NewAssistantMessage(taskID,
				WithContent(&v1.MessagePart{
					Data: &v1.MessagePart_Text_{
						Text: &v1.MessagePart_Text{
							Content: chunk,
						},
					},
				}),
				WithStatus(v1.ContentStatus_CONTENT_STATUS_PARTIAL),
			))
		}),
		model.WithTools(rt.interpreter),
	)

	if err != nil {
		var providerError *model.ProviderError
		if errors.As(err, &providerError) {
			if retryable, retryAfter := providerError.Retryable(); retryable {
				return Result{RetryAfter: retryAfter}, err
			}
			return Result{}, err
		}
		return Result{}, err
	}

	newMessage, err := rt.saveResponse(ctx, taskID, nextMessage, message, agent.Edges.Model)
	if err != nil {
		return Result{}, err
	}

	protoMessage, err := ConvertMemoryMessageToProto(newMessage)
	if err != nil {
		return Result{}, err
	}
	protoMessage.Status.IsFinalResponse = !hasToolCalls(message.Content)
	protoMessage.Status.ContentState = v1.ContentStatus_CONTENT_STATUS_COMPLETE

	rt.eventHub.Publish(taskID, &v1.SubscribeResponse{
		Event: &v1.SubscribeResponse_Message{
			Message: protoMessage,
		},
	})

	toolResults, toolStats, err := rt.callTools(ctx, task, message.Content)
	if err != nil {
		slog.ErrorContext(ctx, "failed to call tools", "error", err)
	}

	if len(toolResults) > 0 {
		_, err := rt.saveToolResults(ctx, taskID, toolResults)
		if err != nil {
			return Result{}, err
		}

		for tool, count := range toolStats {
			task.ToolUses[tool] += count
		}

		_, err = task.Update().SetToolUses(task.ToolUses).Save(ctx)
		if err != nil {
			return Result{}, err
		}
	}

	_, err = rt.memory.Task.UpdateOneID(taskID).AddTurns(1).Save(ctx)
	if err != nil {
		return Result{}, err
	}

	return Result{Retry: true}, nil
}

func (rt *Runtime) saveToolResults(ctx context.Context, taskID uuid.UUID, toolResults []base.ToolResult) (*memory.Message, error) {
	if len(toolResults) > 0 {
		jsonResults, err := json.MarshalIndent(toolResults, "", "  ")
		if err == nil {
			os.WriteFile("/tmp/tool_results.json", jsonResults, 0644)
		}
	}

	toolBlocks := make([]types.MessageBlock, 0, len(toolResults))
	for _, result := range toolResults {
		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		switch result := result.(type) {
		case *codeact.InterpreterToolResult:
			toolBlocks = append(toolBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindCodeInterpreterResult,
				Payload: string(jsonResult),
			})
		case *native.NativeToolResult:
			toolBlocks = append(toolBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindNativeToolResult,
				Payload: string(jsonResult),
			})
		default:
			return nil, fmt.Errorf("unknown tool result type: %T", result)
		}
	}

	return memory.Transaction(ctx, rt.memory, func(tx *memory.Client) (*memory.Message, error) {
		return tx.Message.Create().
			SetTaskID(taskID).
			SetSource(types.MessageSourceSystem).
			SetContent(&types.MessageContent{
				Blocks: toolBlocks,
			}).
			Save(ctx)
	})
}

func (rt *Runtime) fetchTaskWithAgent(ctx context.Context, taskID uuid.UUID) (*memory.Task, *memory.Agent, error) {
	task, err := rt.memory.Task.Query().Where(memory_task.IDEQ(taskID)).WithAgent(func(query *memory.AgentQuery) {
		query.WithModel()
	}).Only(ctx)
	if err != nil {
		return nil, nil, err
	}

	if task.AgentID == uuid.Nil {
		return nil, nil, fmt.Errorf("task has no agent: %s", taskID)
	}

	return task, task.Edges.Agent, nil
}

func (rt *Runtime) fetchTaskMessages(ctx context.Context, taskID uuid.UUID) ([]*memory.Message, *memory.Message, error) {
	messages, err := rt.memory.Message.Query().
		Where(memory_message.TaskIDEQ(taskID)).
		Order(memory_message.ByCreateTime()).
		All(ctx)
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

func (rt *Runtime) createModelProviderClient(ctx context.Context, agent *memory.Agent) (model.ModelProvider, error) {
	m, err := rt.memory.Model.Query().Where(memory_model.IDEQ(agent.ModelID)).WithModelProvider().Only(ctx)
	if err != nil {
		return nil, err
	}

	providerAPI, err := rt.modelProviderClient(m)
	if err != nil {
		return nil, err
	}

	return providerAPI, nil
}

func (rt *Runtime) prepareModelData(
	processedMessages []*memory.Message,
	nextMessage *memory.Message,
) ([]*model.Message, error) {
	modelMessages := make([]*model.Message, 0, len(processedMessages))
	for _, msg := range processedMessages {
		modelMsg, err := ConvertMemoryMessageToModel(msg)
		if err != nil {
			return nil, err
		}
		modelMessages = append(modelMessages, modelMsg)
	}

	modelMsg, err := ConvertMemoryMessageToModel(nextMessage)
	if err != nil {
		return nil, err
	}
	modelMessages = append(modelMessages, modelMsg)

	return modelMessages, nil
}

func (rt *Runtime) assembleSystemPrompt(ctx context.Context, agentInstruction string, cwd string) (string, error) {
	var toolInstruction string
	if len(rt.interpreter.Tools) != 0 {
		toolInstruction = prompt.ToolInstructions()
	}

	var builder strings.Builder
	for _, tool := range rt.interpreter.Tools {
		fmt.Fprintf(&builder, "# %s\n%s\n\n", tool.Name(), tool.Description())
	}

	projectStructure, err := ProjectStructure(cwd)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get project structure", "error", err)
	}

	shell, err := DefaultShell()
	if err != nil {
		slog.ErrorContext(ctx, "failed to get user shell", "error", err)
	}

	devTools := AvailableDevTools()

	tmplParams := struct {
		CurrentTime      string
		WorkingDirectory string
		OperatingSystem  string
		DefaultShell     string
		ProjectStructure string
		ToolInstructions string
		Tools            string
		DevTools         *DevTools
	}{
		WorkingDirectory: cwd,
		OperatingSystem:  runtime.GOOS,
		DefaultShell:     shell.Name,
		ProjectStructure: projectStructure,
		ToolInstructions: toolInstruction,
		Tools:            builder.String(),
		DevTools:         devTools,
	}

	tmpl, err := template.New("system_prompt").Parse(agentInstruction)
	if err != nil {
		return "", err
	}

	builder.Reset()
	err = tmpl.Execute(&builder, tmplParams)
	if err != nil {
		return "", err
	}

	return builder.String(), nil
}

func (rt *Runtime) saveResponse(ctx context.Context, taskID uuid.UUID, processedMessage *memory.Message, msg *model.Message, m *memory.Model) (*memory.Message, error) {
	newMessage, err := memory.Transaction(ctx, rt.memory, func(tx *memory.Client) (*memory.Message, error) {
		_, err := processedMessage.Update().SetProcessedTime(time.Now()).Save(ctx)
		if err != nil {
			return nil, err
		}

		memoryContent, err := ConvertModelContentBlocksToMemory(msg.Content)
		if err != nil {
			return nil, err
		}

		cost := calculateCost(msg.Usage, m)

		t := time.Now()
		message, err := tx.Message.Create().
			SetTaskID(taskID).
			SetSource(types.MessageSourceAssistant).
			SetContent(memoryContent).
			SetProcessedTime(t).
			SetUsage(&types.MessageUsage{
				InputTokens:      msg.Usage.InputTokens,
				OutputTokens:     msg.Usage.OutputTokens,
				CacheWriteTokens: msg.Usage.CacheWriteTokens,
				CacheReadTokens:  msg.Usage.CacheReadTokens,
				Cost:             cost,
			}).
			Save(ctx)

		if err != nil {
			return nil, err
		}

		_, err = tx.Task.UpdateOneID(taskID).
			AddInputTokens(msg.Usage.InputTokens).
			AddOutputTokens(msg.Usage.OutputTokens).
			AddCacheWriteTokens(msg.Usage.CacheWriteTokens).
			AddCacheReadTokens(msg.Usage.CacheReadTokens).
			AddCost(cost).
			Save(ctx)

		if err != nil {
			return nil, err
		}

		return message, nil
	})

	if err != nil {
		return nil, err
	}

	return newMessage, nil
}

func (rt *Runtime) callTools(ctx context.Context, task *memory.Task, content []model.ContentBlock) ([]base.ToolResult, map[string]int64, error) {
	var toolResults []base.ToolResult
	toolStats := make(map[string]int64)

	for _, block := range content {
		toolCall, ok := block.(*model.ToolCallBlock)
		if !ok {
			continue
		}

		switch toolCall.Tool {
		case base.ToolNameCodeInterpreter:
			logInterpreterArgs(ctx, task.ID, toolCall.Args)
			result, err := rt.interpreter.Interpret(ctx, afero.NewOsFs(), toolCall.Args, &codeact.Task{
				ID:               task.ID,
				ProjectDirectory: task.ProjectDirectory,
			})
			toolResults = append(toolResults, &codeact.InterpreterToolResult{
				ID:            toolCall.ID,
				Output:        result.ConsoleOutput,
				FunctionCalls: result.FunctionCalls,
				Error:         conv.ErrorToString(err),
			})

			for tool, count := range result.ToolStats {
				toolStats[tool] += count
			}
			logInterpreterResult(ctx, task.ID, result)
		default:
			slog.WarnContext(ctx, "model requested unknown tool", "tool", toolCall.Tool)
			continue
		}
	}

	return toolResults, toolStats, nil
}

func logInterpreterArgs(ctx context.Context, taskID uuid.UUID, args json.RawMessage) {
	var a codeact.InterpreterArgs
	err := json.Unmarshal(args, &a)
	if err != nil {
		slog.ErrorContext(ctx, "failed to unmarshal interpreter args", "error", err)
		return
	}

	logInterpreter(ctx, taskID, a.Script, "args_interpreter")
}

func logInterpreterResult(ctx context.Context, taskID uuid.UUID, result *codeact.InterpreterResult) {
	jsonResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal interpreter result", "error", err)
		return
	}

	logInterpreter(ctx, taskID, string(jsonResult), "result_interpreter")
}

func logInterpreter(ctx context.Context, taskID uuid.UUID, content string, operation string) {
	taskDir := fmt.Sprintf("/tmp/tool_call/%s", taskID.String())
	if _, err := os.Stat(taskDir); os.IsNotExist(err) {
		err = os.MkdirAll(taskDir, 0755)
		if err != nil {
			slog.ErrorContext(ctx, "failed to create task directory", "error", err)
			return
		}
	}

	fp, err := os.OpenFile(fmt.Sprintf("%s/%s.json", taskDir, operation), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		slog.ErrorContext(ctx, "failed to open file", "error", err)
		return
	}
	defer fp.Close()

	_, err = fp.WriteString(content + "\n\n" + strings.Repeat("-", 100) + "\n\n")
	if err != nil {
		slog.ErrorContext(ctx, "failed to write interpreter args", "error", err)
	}
}

func (rt *Runtime) modelProviderClient(m *memory.Model) (model.ModelProvider, error) {
	if m.Edges.ModelProvider == nil {
		return nil, fmt.Errorf("model provider not found")
	}
	provider := m.Edges.ModelProvider

	providerAuth, err := rt.encryption.Decrypt(provider.Secret, []byte(secret.ModelProviderSecret(provider.ID)))
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

	switch provider.ProviderType {
	case types.ModelProviderTypeAnthropic:
		provider, err := model.NewAnthropicProvider(auth.APIKey)
		if err != nil {
			return nil, err
		}
		return provider, nil
	case types.ModelProviderTypeOpenAI:
		provider, err := model.NewOpenAICompletionProvider(auth.APIKey)
		if err != nil {
			return nil, err
		}
		return provider, nil
	case types.ModelProviderTypeGemini:
		provider, err := model.NewGeminiProvider(auth.APIKey)
		if err != nil {
			return nil, err
		}
		return provider, nil
	case types.ModelProviderTypeXAI:
		provider, err := model.NewOpenAICompletionProvider(auth.APIKey, model.WithURL("https://api.xai.com/v1"))
		if err != nil {
			return nil, err
		}
		return provider, nil
	default:
		return nil, fmt.Errorf("unknown model provider type: %s", provider.ProviderType)
	}
}

func calculateCost(usage model.Usage, model *memory.Model) float64 {
	return (float64(usage.InputTokens) * model.InputCost / 1000000) +
		(float64(usage.OutputTokens) * model.OutputCost / 1000000) +
		(float64(usage.CacheWriteTokens) * model.CacheWriteCost / 1000000) +
		(float64(usage.CacheReadTokens) * model.CacheReadCost / 1000000)
}

func hasToolCalls(content []model.ContentBlock) bool {
	for _, block := range content {
		if _, ok := block.(*model.ToolCallBlock); ok {
			return true
		}
	}

	return false
}

func (rt *Runtime) publishMessage(taskID uuid.UUID, message *v1.Message) {
	rt.eventHub.Publish(taskID, &v1.SubscribeResponse{
		Event: &v1.SubscribeResponse_Message{
			Message: message,
		},
	})
}

func (rt *Runtime) publishTaskEvent(taskID uuid.UUID) {
	taskEvent := &v1.TaskEvent{
		TaskId:    taskID.String(),
		Timestamp: timestamppb.Now(),
	}

	rt.eventHub.Publish(taskID, &v1.SubscribeResponse{
		Event: &v1.SubscribeResponse_TaskEvent{
			TaskEvent: taskEvent,
		},
	})
}

func (rt *Runtime) setTaskPhaseAndPublish(ctx context.Context, taskID uuid.UUID, phase types.TaskPhase) {
	_, err := memory.Transaction(ctx, rt.memory, func(tx *memory.Client) (*memory.Task, error) {
		return tx.Task.UpdateOneID(taskID).SetPhase(types.TaskPhase(phase)).Save(ctx)
	})

	if err != nil {
		slog.Error("failed to set task phase", "error", err)
	}

	rt.publishTaskEvent(taskID)
}

func (rt *Runtime) Encryption() *secret.Client {
	return rt.encryption
}

func (rt *Runtime) Memory() *memory.Client {
	return rt.memory
}

func (rt *Runtime) EventHub() *event.MessageHub {
	return rt.eventHub
}

func (rt *Runtime) CancelTask(taskID uuid.UUID) {
	cancel, ok := rt.runningTasks.Get(taskID)
	if !ok {
		return
	}

	cancel()
}

type SyncMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func NewSyncMap[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m:  make(map[K]V),
		mu: sync.RWMutex{},
	}
}

func (sm *SyncMap[K, V]) Get(key K) (V, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	val, ok := sm.m[key]
	return val, ok
}

func (sm *SyncMap[K, V]) Set(key K, value V) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m[key] = value
}

func (sm *SyncMap[K, V]) Delete(key K) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.m, key)
}

func (sm *SyncMap[K, V]) Len() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.m)
}

func WithRole(role v1.MessageRole) func(*v1.Message) {
	return func(msg *v1.Message) {
		msg.Metadata.Role = role
	}
}

func WithContent(content *v1.MessagePart) func(*v1.Message) {
	return func(msg *v1.Message) {
		msg.Spec.Content = append(msg.Spec.Content, content)
	}
}

func WithStatus(status v1.ContentStatus) func(*v1.Message) {
	return func(msg *v1.Message) {
		msg.Status.ContentState = status
	}
}

func NewUserMessage(taskID uuid.UUID, options ...func(*v1.Message)) *v1.Message {
	msg := NewMessage(taskID, WithRole(v1.MessageRole_MESSAGE_ROLE_USER))

	for _, option := range options {
		option(msg)
	}

	return msg
}

func NewAssistantMessage(taskID uuid.UUID, options ...func(*v1.Message)) *v1.Message {
	msg := NewMessage(taskID, WithRole(v1.MessageRole_MESSAGE_ROLE_ASSISTANT))

	for _, option := range options {
		option(msg)
	}

	return msg
}

func NewSystemMessage(taskID uuid.UUID, options ...func(*v1.Message)) *v1.Message {
	msg := NewMessage(taskID, WithRole(v1.MessageRole_MESSAGE_ROLE_SYSTEM))

	for _, option := range options {
		option(msg)
	}

	return msg
}

func NewMessage(taskID uuid.UUID, options ...func(*v1.Message)) *v1.Message {
	msg := &v1.Message{
		Metadata: &v1.MessageMetadata{
			Id:        uuid.New().String(),
			TaskId:    taskID.String(),
			CreatedAt: timestamppb.New(time.Now()),
			UpdatedAt: timestamppb.New(time.Now()),
			Role:      v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
		},
		Spec: &v1.MessageSpec{},
		Status: &v1.MessageStatus{
			ContentState:    v1.ContentStatus_CONTENT_STATUS_COMPLETE,
			IsFinalResponse: false,
		},
	}

	for _, option := range options {
		option(msg)
	}

	return msg
}
