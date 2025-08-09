package agent

import (
	"context"
	"encoding/json"
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
	"github.com/furisto/construct/backend/api"
	"github.com/furisto/construct/backend/memory"
	memory_message "github.com/furisto/construct/backend/memory/message"
	memory_model "github.com/furisto/construct/backend/memory/model"
	"github.com/furisto/construct/backend/memory/schema/types"
	memory_task "github.com/furisto/construct/backend/memory/task"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/prompt"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/stream"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/furisto/construct/backend/tool/native"
	"github.com/furisto/construct/shared/conv"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/client-go/util/workqueue"
)

const DefaultServerPort = 29333

type RuntimeOptions struct {
	Tools       []codeact.Tool
	Concurrency int
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

type Runtime struct {
	api         *api.Server
	memory      *memory.Client
	encryption  *secret.Client
	eventHub    *stream.EventHub
	concurrency int
	queue       workqueue.TypedDelayingInterface[uuid.UUID]
	running     atomic.Bool
	interpreter *codeact.Interpreter
}

func NewRuntime(memory *memory.Client, encryption *secret.Client, listener net.Listener, opts ...RuntimeOption) (*Runtime, error) {
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

	interceptors := []codeact.Interceptor{
		codeact.InterceptorFunc(codeact.ToolStatisticsInterceptor),
		codeact.InterceptorFunc(codeact.DurableFunctionInterceptor),
		codeact.NewToolEventPublisher(messageHub),
		codeact.InterceptorFunc(codeact.ResetTemporarySessionValuesInterceptor),
	}

	runtime := &Runtime{
		memory:     memory,
		encryption: encryption,

		eventHub:    messageHub,
		concurrency: options.Concurrency,
		queue:       queue,
		interpreter: codeact.NewInterpreter(options.Tools, interceptors),
	}

	api := api.NewServer(runtime, listener)
	runtime.api = api

	return runtime, nil
}

func (rt *Runtime) Run(ctx context.Context) error {
	if !rt.running.CompareAndSwap(false, true) {
		return nil
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := rt.api.ListenAndServe(ctx)
		if err != nil && err != http.ErrServerClosed {
			slog.Error("API server failed", "error", err)
		}
	}()

	for range rt.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				taskID, shutdown := rt.queue.Get()
				if shutdown {
					return
				}
				err := rt.processTask(ctx, taskID)
				if err != nil {
					slog.Error("failed to process task", "error", err)
				}
			}
		}()
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	err := rt.api.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error("failed to shutdown API server", "error", err)
	}

	rt.queue.ShutDownWithDrain()

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

func (rt *Runtime) processTask(ctx context.Context, taskID uuid.UUID) error {
	defer rt.queue.Done(taskID)

	task, agent, err := rt.fetchTaskWithAgent(ctx, taskID)
	if err != nil {
		return err
	}

	processedMessages, nextMessage, err := rt.fetchTaskMessages(ctx, task.ID)
	if err != nil {
		return err
	}

	if nextMessage == nil {
		slog.DebugContext(ctx, "no unprocessed messages, skipping", "task_id", taskID)
		return nil
	}

	modelProvider, err := rt.createModelProviderClient(ctx, agent)
	if err != nil {
		return err
	}

	modelMessages, err := rt.prepareModelData(processedMessages, nextMessage)
	if err != nil {
		return err
	}

	systemPrompt, err := rt.assembleSystemPrompt(agent.Instructions, task.ProjectDirectory)
	if err != nil {
		return err
	}

	messageID := uuid.New()
	message, err := modelProvider.InvokeModel(
		ctx,
		agent.Edges.Model.Name,
		systemPrompt,
		modelMessages,
		model.WithStreamHandler(func(ctx context.Context, message *model.Message) {
			for _, block := range message.Content {
				switch block := block.(type) {
				case *model.TextBlock:
					rt.eventHub.Publish(taskID, &v1.SubscribeResponse{
						Message: &v1.Message{
							Metadata: &v1.MessageMetadata{
								Id:        messageID.String(),
								TaskId:    taskID.String(),
								CreatedAt: timestamppb.New(time.Now()),
								UpdatedAt: timestamppb.New(time.Now()),
								AgentId:   conv.Ptr(agent.ID.String()),
								ModelId:   conv.Ptr(agent.Edges.Model.ID.String()),
								Role:      v1.MessageRole_MESSAGE_ROLE_ASSISTANT,
							},
							Spec: &v1.MessageSpec{
								Content: []*v1.MessagePart{
									{
										Data: &v1.MessagePart_Text_{
											Text: &v1.MessagePart_Text{
												Content: block.Text,
											},
										},
									},
								},
							},
							Status: &v1.MessageStatus{
								ContentState:    v1.ContentStatus_CONTENT_STATUS_PARTIAL,
								IsFinalResponse: false,
							},
						},
					})
				case *model.ToolCallBlock:
					fmt.Println(block.Args)
				}
			}

		}),
		model.WithTools(rt.interpreter),
	)

	if err != nil {
		return err
	}

	newMessage, err := rt.saveResponse(ctx, taskID, nextMessage, message, agent.Edges.Model)
	if err != nil {
		return err
	}

	protoMessage, err := ConvertMemoryMessageToProto(newMessage)
	if err != nil {
		return err
	}
	protoMessage.Status.IsFinalResponse = !hasToolCalls(message.Content)
	protoMessage.Status.ContentState = v1.ContentStatus_CONTENT_STATUS_COMPLETE

	rt.eventHub.Publish(taskID, &v1.SubscribeResponse{
		Message: protoMessage,
	})

	toolResults, toolStats, err := rt.callTools(ctx, taskID, message.Content)
	if err != nil {
		slog.Error("failed to call tools", "error", err)
	}

	if len(toolResults) > 0 {
		_, err := rt.saveToolResults(ctx, taskID, toolResults)
		if err != nil {
			return err
		}

		for tool, count := range toolStats {
			task.ToolUses[tool] += count
		}

		_, err = task.Update().SetToolUses(task.ToolUses).Save(ctx)
		if err != nil {
			return err
		}
	}

	_, err = rt.memory.Task.UpdateOneID(taskID).AddTurns(1).Save(ctx)
	if err != nil {
		return err
	}

	rt.TriggerReconciliation(taskID)

	return nil
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

	return rt.memory.Message.Create().
		SetTaskID(taskID).
		SetSource(types.MessageSourceSystem).
		SetContent(&types.MessageContent{
			Blocks: toolBlocks,
		}).
		Save(ctx)
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
	m, err := rt.memory.Model.Query().Where(memory_model.IDEQ(agent.DefaultModel)).WithModelProvider().Only(ctx)
	if err != nil {
		return nil, err
	}

	providerAPI, err := rt.modelProviderAPI(m)
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

func (rt *Runtime) assembleSystemPrompt(agentInstruction string, cwd string) (string, error) {
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
		slog.Error("failed to get project structure", "error", err)
	}

	shell, err := DefaultShell()
	if err != nil {
		slog.Error("failed to get user shell", "error", err)
	}

	tmplParams := struct {
		WorkingDirectory string
		OperatingSystem  string
		DefaultShell     string
		ProjectStructure string
		ToolInstructions string
		Tools            string
	}{
		WorkingDirectory: cwd,
		OperatingSystem:  runtime.GOOS,
		DefaultShell:     shell.Name,
		ProjectStructure: projectStructure,
		ToolInstructions: toolInstruction,
		Tools:            builder.String(),
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
	newMessage, err := rt.memory.Message.Create().
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

	_, err = rt.memory.Task.UpdateOneID(taskID).
		AddInputTokens(msg.Usage.InputTokens).
		AddOutputTokens(msg.Usage.OutputTokens).
		AddCacheWriteTokens(msg.Usage.CacheWriteTokens).
		AddCacheReadTokens(msg.Usage.CacheReadTokens).
		AddCost(cost).
		Save(ctx)

	if err != nil {
		return nil, err
	}

	return newMessage, nil
}

func (rt *Runtime) callTools(ctx context.Context, taskID uuid.UUID, content []model.ContentBlock) ([]base.ToolResult, map[string]int64, error) {
	var toolResults []base.ToolResult
	toolStats := make(map[string]int64)

	for _, block := range content {
		toolCall, ok := block.(*model.ToolCallBlock)
		if !ok {
			continue
		}

		switch toolCall.Tool {
		case base.ToolNameCodeInterpreter:
			os.WriteFile("/tmp/tool_call.json", []byte(toolCall.Args), 0644)
			result, err := rt.interpreter.Interpret(ctx, afero.NewOsFs(), toolCall.Args, taskID)
			toolResults = append(toolResults, &codeact.InterpreterToolResult{
				ID:            toolCall.ID,
				Output:        result.ConsoleOutput,
				FunctionCalls: result.FunctionCalls,
				Error:         conv.ErrorToString(err),
			})

			for tool, count := range result.ToolStats {
				toolStats[tool] += count
			}
		default:
			slog.WarnContext(ctx, "model requested unknown tool", "tool", toolCall.Tool)
			continue
		}
	}

	return toolResults, toolStats, nil
}

func (rt *Runtime) modelProviderAPI(m *memory.Model) (model.ModelProvider, error) {
	if m.Edges.ModelProvider == nil {
		return nil, fmt.Errorf("model provider not found")
	}
	provider := m.Edges.ModelProvider

	switch provider.ProviderType {
	case types.ModelProviderTypeAnthropic:
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

func hasToolCalls(content []model.ContentBlock) bool {
	for _, block := range content {
		if _, ok := block.(*model.ToolCallBlock); ok {
			return true
		}
	}

	return false
}

func (rt *Runtime) Encryption() *secret.Client {
	return rt.encryption
}

func (rt *Runtime) Memory() *memory.Client {
	return rt.memory
}

func (rt *Runtime) TriggerReconciliation(taskID uuid.UUID) {
	rt.queue.Add(taskID)
}

func (rt *Runtime) EventHub() *stream.EventHub {
	return rt.eventHub
}
