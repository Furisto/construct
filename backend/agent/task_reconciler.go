package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	memory_message "github.com/furisto/construct/backend/memory/message"
	"github.com/furisto/construct/backend/memory/schema/types"
	memory_task "github.com/furisto/construct/backend/memory/task"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/prompt"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/furisto/construct/shared/conv"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"golang.org/x/sync/singleflight"
	"k8s.io/client-go/util/workqueue"
)

const ToolExecutionInProgressMarker = "__TOOL_EXECUTION_IN_PROGRESS__"

type Result struct {
	RetryAfter time.Duration
	Retry      bool
}

type TaskStatus struct {
	Phase             TaskPhase
	NextMessage       *memory.Message
	ProcessedMessages []*memory.Message
}

type TaskPhase string

const (
	TaskPhaseAwaitInput   TaskPhase = "await_input"
	TaskPhaseExecuteTools TaskPhase = "execute_tools"
	TaskPhaseInvokeModel  TaskPhase = "invoke_model"
	TaskPhaseSuspended    TaskPhase = "suspended"
)

type TaskReconciler struct {
	memory          *memory.Client
	interpreter     *codeact.Interpreter
	bus             *event.Bus
	queue           workqueue.TypedDelayingInterface[uuid.UUID]
	providerFactory *ModelProviderFactory
	concurrency     int
	runningTasks    *SyncMap[uuid.UUID, context.CancelFunc]
	titleGenGroup   singleflight.Group
	wg              sync.WaitGroup
	logger          *slog.Logger
}

func NewTaskReconciler(
	memory *memory.Client,
	interpreter *codeact.Interpreter,
	concurrency int,
	bus *event.Bus,
	providerFactory *ModelProviderFactory,
	metricsRegistry prometheus.Registerer,
) *TaskReconciler {
	wqProvider := newWorkqueueMetricsProvider(metricsRegistry)
	workqueue.SetProvider(wqProvider)

	queue := workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[uuid.UUID]{
		Name: "construct",
	})
	return &TaskReconciler{
		memory:          memory,
		interpreter:     interpreter,
		bus:             bus,
		providerFactory: providerFactory,
		queue:           queue,
		concurrency:     concurrency,
		runningTasks:    NewSyncMap[uuid.UUID, context.CancelFunc](),
		logger:          slog.With(KeyComponent, "task_reconciler"),
	}
}

func (r *TaskReconciler) Run(ctx context.Context) error {
	LogComponentStartup(r.logger, "task reconciler",
		KeyConcurrency, r.concurrency,
	)

	for range r.concurrency {
		r.wg.Add(1)
		go func() {
			r.worker(ctx)
		}()
	}

	taskEventSub := event.Subscribe(r.bus, func(ctx context.Context, e event.TaskReconciliationEvent) {
		r.queue.Add(e.TaskID)
	}, nil)

	taskSuspendedEventSub := event.Subscribe(r.bus, func(ctx context.Context, e event.TaskSuspendedEvent) {
		cancel, ok := r.runningTasks.Get(e.TaskID)
		if ok {
			r.logger.DebugContext(ctx, "task suspension signal received",
				KeyTaskID, e.TaskID,
			)
			cancel()
		}
	}, nil)

	r.logger.InfoContext(ctx, "task reconciler initialization complete")
	<-ctx.Done()
	r.logger.InfoContext(ctx, "task reconciler shutdown initiated")
	shutdownStart := time.Now()

	taskEventSub.Unsubscribe()
	taskSuspendedEventSub.Unsubscribe()

	r.queue.ShutDownWithDrain()
	r.logger.DebugContext(ctx, "task queue shutdown with drain complete")

	stop := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(stop)
	}()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case <-stop:
		LogComponentShutdown(r.logger, "task reconciler", shutdownStart)
		return nil
	case <-shutdownCtx.Done():
		err := shutdownCtx.Err()
		r.logger.Error("task reconciler shutdown timeout", "error", err)
		return err
	}
}

func (r *TaskReconciler) worker(ctx context.Context) {
	defer r.wg.Done()

	for {
		taskID, shutdown := r.queue.Get()
		if shutdown {
			r.logger.DebugContext(ctx, "worker shutdown signal received")
			return
		}

		result, err := r.reconcile(ctx, taskID)
		if err != nil {
			r.logger.ErrorContext(ctx, "task reconciliation failed",
				KeyTaskID, taskID,
				"error", err,
			)
			r.publishError(ctx, err, taskID)
		}

		switch {
		case result.RetryAfter > 0:
			r.logger.DebugContext(ctx, "scheduling task retry",
				KeyTaskID, taskID,
				"retry_after_ms", result.RetryAfter.Milliseconds(),
			)
			r.queue.AddAfter(taskID, result.RetryAfter)
		case result.Retry:
			r.logger.DebugContext(ctx, "scheduling immediate retry",
				KeyTaskID, taskID,
			)
			r.queue.Add(taskID)
		}

		r.queue.Done(taskID)
	}
}

func (r *TaskReconciler) publishError(ctx context.Context, err error, taskID uuid.UUID) {
	if errors.Is(err, context.Canceled) {
		return
	}

	r.logger.InfoContext(ctx, "publishing error message",
		KeyTaskID, taskID,
		KeyError, err.Error(),
	)

	event.Publish(r.bus, event.ErrorEvent{
		TaskID: taskID,
		Error:  err,
	})
}

// Reconcile is the main entry point for reconciling a task's conversation state
func (r *TaskReconciler) reconcile(ctx context.Context, taskID uuid.UUID) (Result, error) {
	logger := r.logger.With(KeyTaskID, taskID)

	defer func() {
		if rec := recover(); rec != nil {
			logger.Error("panic in reconcile",
				"error", rec,
			)
		}
	}()

	reconcileStart := time.Now()
	logger.DebugContext(ctx, "reconciliation started")

	ctx, cancel := context.WithCancel(ctx)
	r.runningTasks.Set(taskID, cancel)
	defer r.runningTasks.Delete(taskID)
	defer cancel()

	task, agent, err := r.fetchTaskWithAgent(ctx, taskID)
	if err != nil {
		LogError(logger, "failed to fetch task with agent", err)
		return Result{}, fmt.Errorf("failed to fetch task: %w", err)
	}
	logger.DebugContext(ctx, "task and agent fetched",
		KeyAgentID, agent.ID,
		KeyModel, agent.Edges.Model.Name,
	)

	messages, err := r.memory.Message.Query().
		Where(memory_message.TaskIDEQ(taskID)).
		Order(memory_message.ByCreateTime()).
		All(ctx)
	if err != nil {
		LogError(logger, "failed to fetch messages", err)
		return Result{}, fmt.Errorf("failed to fetch messages: %w", err)
	}
	logger.DebugContext(ctx, "messages fetched",
		KeyMessageCount, len(messages),
	)

	if shouldGenerateTitle(task, messages) {
		logger.DebugContext(ctx, "scheduling async title generation")
		go r.generateTitle(taskID)
	}

	status, err := r.computeStatus(task, messages)
	if err != nil {
		LogError(logger, "failed to compute status", err)
		return Result{}, fmt.Errorf("failed to compute status: %w", err)
	}

	logger.DebugContext(ctx, "task status computed",
		KeyPhase, string(status.Phase),
		KeyProcessedCount, len(status.ProcessedMessages),
	)

	r.setTaskPhaseAndPublish(ctx, taskID, status.Phase)

	switch status.Phase {
	case TaskPhaseAwaitInput:
		logger.DebugContext(ctx, "no unprocessed messages, awaiting input")
		LogOperationEnd(logger, "reconciliation (await_input)", reconcileStart)
		return Result{}, nil

	case TaskPhaseSuspended:
		logger.DebugContext(ctx, "task is suspended")
		LogOperationEnd(logger, "reconciliation (suspended)", reconcileStart)
		return Result{}, nil

	case TaskPhaseInvokeModel:
		return r.reconcileInvokeModel(ctx, taskID, task, agent, status)

	case TaskPhaseExecuteTools:
		return r.reconcileExecuteTools(ctx, taskID, task, status)

	default:
		logger.ErrorContext(ctx, "unknown phase",
			KeyPhase, string(status.Phase),
		)
		return Result{}, fmt.Errorf("unknown phase: %s", status.Phase)
	}
}

func (r *TaskReconciler) fetchTaskWithAgent(ctx context.Context, taskID uuid.UUID) (*memory.Task, *memory.Agent, error) {
	task, err := r.memory.Task.Query().
		Where(memory_task.IDEQ(taskID)).
		WithAgent(func(query *memory.AgentQuery) {
			query.WithModel()
		}).
		Only(ctx)

	if err != nil {
		return nil, nil, err
	}

	if task.Edges.Agent == nil {
		return nil, nil, fmt.Errorf("no agent associated with task: %s", taskID)
	}

	return task, task.Edges.Agent, nil
}

// computeStatus analyzes the message history and determines what action to take
func (r *TaskReconciler) computeStatus(task *memory.Task, messages []*memory.Message) (*TaskStatus, error) {
	if task.DesiredPhase == types.TaskPhaseSuspended {
		return &TaskStatus{Phase: TaskPhaseSuspended}, nil
	}

	if len(messages) == 0 {
		return &TaskStatus{Phase: TaskPhaseAwaitInput}, nil
	}

	categorized := map[string][]*memory.Message{
		"processed":            make([]*memory.Message, 0),
		"unprocessedUser":      make([]*memory.Message, 0),
		"unprocessedAssistant": make([]*memory.Message, 0),
		"unprocessedSystem":    make([]*memory.Message, 0),
	}

	for _, message := range messages {
		if message.Source == types.MessageSourceTask {
			continue
		}

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

	if !hasUnprocessedMessages(categorized) {
		return &TaskStatus{Phase: TaskPhaseAwaitInput, ProcessedMessages: categorized["processed"]}, nil
	}

	taskStatus := &TaskStatus{
		ProcessedMessages: categorized["processed"],
	}

	switch {
	case len(categorized["unprocessedSystem"]) > 0:
		taskStatus.Phase = TaskPhaseInvokeModel
		taskStatus.NextMessage = categorized["unprocessedSystem"][0]
	case len(categorized["unprocessedAssistant"]) > 0:
		taskStatus.Phase = TaskPhaseExecuteTools
		taskStatus.NextMessage = categorized["unprocessedAssistant"][0]
	case len(categorized["unprocessedUser"]) > 0:
		taskStatus.Phase = TaskPhaseInvokeModel
		taskStatus.NextMessage = categorized["unprocessedUser"][0]
	}

	return taskStatus, nil
}

func hasUnprocessedMessages(categorized map[string][]*memory.Message) bool {
	return len(categorized["unprocessedUser"]) > 0 || len(categorized["unprocessedAssistant"]) > 0 || len(categorized["unprocessedSystem"]) > 0
}

func (r *TaskReconciler) reconcileInvokeModel(ctx context.Context, taskID uuid.UUID, task *memory.Task, agent *memory.Agent, status *TaskStatus) (Result, error) {
	logger := r.logger.With(
		KeyTaskID, taskID,
		KeyMessageID, status.NextMessage.ID,
		KeyModel, agent.Edges.Model.Name,
		KeyAgentID, agent.ID,
	)
	LogOperationStart(logger, "reconciliation (invoke_model)")

	reconcileStart := time.Now()
	logger.InfoContext(ctx, "model invocation phase started")

	if status.NextMessage.Source == types.MessageSourceUser {
		msg, err := ConvertMemoryMessageToProto(status.NextMessage)
		if err != nil {
			LogError(logger, "failed to convert user message", err)
			return Result{}, err
		}
		r.publishMessage(taskID, msg)
		logger.DebugContext(ctx, "user message published")
	}

	modelMessages, err := r.buildMessageHistory(status.ProcessedMessages, status.NextMessage)
	if err != nil {
		LogError(logger, "failed to build message history", err)
		return Result{}, fmt.Errorf("failed to prepare model messages: %w", err)
	}
	logger.DebugContext(ctx, "message history built",
		"history_length", len(modelMessages),
	)

	modelProvider, err := r.providerFactory.CreateClient(ctx, agent.Edges.Model.ModelProviderID)
	if err != nil {
		LogError(logger, "failed to create model provider", err)
		return Result{}, fmt.Errorf("failed to create model provider: %w", err)
	}

	systemPrompt, err := r.assembleSystemPrompt(ctx, agent.Instructions, task.ProjectDirectory)
	if err != nil {
		LogError(logger, "failed to assemble system prompt", err)
		return Result{}, fmt.Errorf("failed to assemble system prompt: %w", err)
	}

	LogOperationStart(logger, "invoke model")
	invokeStart := time.Now()

	messageID := uuid.New()
	message, err := modelProvider.InvokeModel(
		ctx,
		agent.Edges.Model.Name,
		systemPrompt,
		modelMessages,
		model.WithTools(r.interpreter),
		model.WithStreamHandler(func(ctx context.Context, chunk string) {
			r.publishDeltaMessage(taskID, messageID, chunk)
		}),
	)
	LogOperationEnd(logger, "invoke model", invokeStart)

	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.InfoContext(ctx, "model invocation cancelled by user")
			_, err = memory.Transaction(ctx, r.memory, func(tx *memory.Client) (*memory.Message, error) {
				err := r.markMessageAsProcessed(ctx, status.NextMessage)
				if err != nil {
					return nil, err
				}
				return nil, nil
			})
			if err != nil {
				LogError(logger, "failed to mark message with cancellation", err)
				return Result{}, fmt.Errorf("failed to mark message with cancellation: %w", err)
			}
			return Result{Retry: false}, nil
		}

		var providerError *model.ProviderError
		if errors.As(err, &providerError) {
			if retryable, retryAfter := providerError.Retryable(); retryable {
				LogError(logger, "model invocation failed with retryable error", err, KeyRetryAfter, retryAfter.Milliseconds())
				return Result{RetryAfter: retryAfter}, err
			}
		}

		return Result{}, err
	}

	cost := calculateCost(message.Usage, agent.Edges.Model)

	LogTokenUsage(logger, slog.LevelInfo,
		message.Usage.InputTokens,
		message.Usage.OutputTokens,
		message.Usage.CacheWriteTokens,
		message.Usage.CacheReadTokens,
		cost,
		time.Since(invokeStart),
	)

	modelMessage, err := memory.Transaction(ctx, r.memory, func(tx *memory.Client) (*memory.Message, error) {
		err = r.markMessageAsProcessed(ctx, status.NextMessage)
		if err != nil {
			return nil, fmt.Errorf("failed to mark message as processed: %w", err)
		}

		modelMessage, err := r.persistModelResponse(ctx, taskID, message, cost)
		if err != nil {
			return nil, fmt.Errorf("failed to persist model response: %w", err)
		}
		return modelMessage, nil
	})

	if err != nil {
		LogError(logger, "failed to persist model response", err)
		return Result{}, fmt.Errorf("failed to persist model response: %w", err)
	}

	protoMessage, err := ConvertMemoryMessageToProto(modelMessage)
	if err != nil {
		LogError(logger, "failed to convert model message to proto", err)
		return Result{}, err
	}
	protoMessage.Status.IsFinalResponse = !hasToolCalls(message.Content)
	protoMessage.Status.ContentState = v1.ContentStatus_CONTENT_STATUS_COMPLETE
	r.publishMessage(taskID, protoMessage)

	LogOperationEnd(logger, "reconciliation (invoke_model)", reconcileStart)

	return Result{Retry: true}, nil
}

func (r *TaskReconciler) buildMessageHistory(processedMessages []*memory.Message, nextMessage *memory.Message) ([]*model.Message, error) {
	modelMessages := make([]*model.Message, 0, len(processedMessages)+1)

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

func (r *TaskReconciler) assembleSystemPrompt(ctx context.Context, agentInstruction string, cwd string) (string, error) {
	var toolInstruction string
	if len(r.interpreter.Tools) != 0 {
		toolInstruction = prompt.ToolInstructions()
	}

	var builder strings.Builder
	for _, tool := range r.interpreter.Tools {
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

func (r *TaskReconciler) persistModelResponse(ctx context.Context, taskID uuid.UUID, modelResponse *model.Message, cost float64) (*memory.Message, error) {
	message, err := memory.Transaction(ctx, r.memory, func(tx *memory.Client) (*memory.Message, error) {
		memoryContent, err := ConvertModelContentBlocksToMemory(modelResponse.Content)
		if err != nil {
			return nil, err
		}

		assistantMsg := tx.Message.Create().
			SetTaskID(taskID).
			SetSource(types.MessageSourceAssistant).
			SetContent(memoryContent).
			SetUsage(&types.MessageUsage{
				InputTokens:      modelResponse.Usage.InputTokens,
				OutputTokens:     modelResponse.Usage.OutputTokens,
				CacheWriteTokens: modelResponse.Usage.CacheWriteTokens,
				CacheReadTokens:  modelResponse.Usage.CacheReadTokens,
				Cost:             cost,
			})

		// If no tool calls, mark as processed immediately
		if !hasToolCalls(modelResponse.Content) {
			assistantMsg = assistantMsg.SetProcessedTime(time.Now())
		}

		savedMsg, err := assistantMsg.Save(ctx)
		if err != nil {
			return nil, err
		}

		_, err = tx.Task.UpdateOneID(taskID).
			AddInputTokens(modelResponse.Usage.InputTokens).
			AddOutputTokens(modelResponse.Usage.OutputTokens).
			AddCacheWriteTokens(modelResponse.Usage.CacheWriteTokens).
			AddCacheReadTokens(modelResponse.Usage.CacheReadTokens).
			AddCost(cost).
			Save(ctx)

		if err != nil {
			return nil, err
		}

		return savedMsg, nil
	})

	return message, err
}

func (r *TaskReconciler) reconcileExecuteTools(ctx context.Context, taskID uuid.UUID, task *memory.Task, status *TaskStatus) (Result, error) {
	logger := r.logger.With(
		KeyTaskID, taskID,
		KeyMessageID, status.NextMessage.ID,
	)
	LogOperationStart(logger, "reconciliation (execute_tools)")

	toolStart := time.Now()
	logger.DebugContext(ctx, "tool execution phase started")

	toolResults, toolStats, err := r.callTools(ctx, task, status.NextMessage)
	if err != nil {
		LogError(logger, "failed to call tools", err)
	}

	_, err = memory.Transaction(ctx, r.memory, func(tx *memory.Client) (*memory.Message, error) {
		err = r.markMessageAsProcessed(ctx, status.NextMessage)
		if err != nil {
			return nil, fmt.Errorf("failed to mark message as processed: %w", err)
		}

		if len(toolResults) > 0 {
			_, persistErr := r.persistToolResults(ctx, taskID, toolResults, tx)
			if persistErr != nil {
				LogError(logger, "failed to persist tool results", persistErr)
				return nil, fmt.Errorf("failed to update message with results: %w", persistErr)
			}

			logger.DebugContext(ctx, "tool results persisted",
				"result_count", len(toolResults),
			)

			// Update task tool usage statistics
			for tool, count := range toolStats {
				task.ToolUses[tool] += count
			}

			_, err = tx.Task.UpdateOneID(taskID).SetToolUses(toolStats).Save(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to update task tool usage: %w", err)
			}

			logger.DebugContext(ctx, "task tool usage updated",
				KeyToolStats, toolStats,
			)

			return nil, nil
		}

		return nil, nil
	})

	logger.InfoContext(ctx, "tool execution completed",
		"result_count", len(toolResults),
	)
	LogOperationEnd(logger, "tool execution", toolStart)

	return Result{Retry: true}, nil
}

func (r *TaskReconciler) callTools(ctx context.Context, task *memory.Task, message *memory.Message) ([]base.ToolResult, map[string]int64, error) {
	logger := r.logger.With(
		KeyTaskID, task.ID,
		KeyMessageID, message.ID,
	)
	LogOperationStart(logger, "call tools")

	var toolResults []base.ToolResult
	toolStats := make(map[string]int64)

	for _, block := range message.Content.Blocks {
		switch block.Kind {
		case types.MessageBlockKindCodeInterpreterCall:
			var toolCall model.ToolCallBlock
			err := json.Unmarshal([]byte(block.Payload), &toolCall)
			if err != nil {
				logger.ErrorContext(ctx, "failed to unmarshal tool call", "error", err)
				return nil, nil, fmt.Errorf("failed to unmarshal tool call: %w", err)
			}
			logInterpreterArgs(ctx, task.ID, toolCall.ID, toolCall.Args)

			toolStart := time.Now()
			result, err := r.interpreter.Interpret(ctx, afero.NewOsFs(), toolCall.Args, &codeact.Task{
				ID:               task.ID,
				ProjectDirectory: task.ProjectDirectory,
				ParentTaskID:     task.ParentTaskID,
			}, r.memory, r.bus)
			toolDuration := time.Since(toolStart)

			if errors.Is(ctx.Err(), context.Canceled) {
				err = errors.New("tool execution was cancelled by user. Wait for further instructions")
			}

			success := err == nil
			if !success {
				LogError(logger, "code interpreter execution failed", err, KeyToolDuration, toolDuration.Milliseconds())
			} else {
				logger.DebugContext(ctx, "code interpreter execution completed",
					"duration_ms", toolDuration.Milliseconds(),
					"success", true,
				)
			}
			interpreterResult := &codeact.InterpreterToolResult{
				ID:            toolCall.ID,
				Output:        result.ConsoleOutput,
				FunctionCalls: result.FunctionCalls,
				Error:         conv.ErrorToString(err),
			}
			toolResults = append(toolResults, interpreterResult)

			for tool, count := range result.ToolStats {
				toolStats[tool] += count
				logger.DebugContext(ctx, "tool invoked",
					KeyToolName, tool,
					"count", count,
				)
			}
			logInterpreterResult(ctx, task.ID, toolCall.ID, interpreterResult)
		}
	}

	logger.DebugContext(ctx, "all tools executed",
		"result_count", len(toolResults),
		KeyToolStats, toolStats,
	)

	return toolResults, toolStats, nil
}

func (r *TaskReconciler) persistToolResults(ctx context.Context, taskID uuid.UUID, toolResults []base.ToolResult, tx *memory.Client) (*memory.Message, error) {
	toolBlocks := make([]types.MessageBlock, 0, len(toolResults))
	for _, result := range toolResults {
		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool result: %w", err)
		}

		switch result.(type) {
		case *codeact.InterpreterToolResult:
			toolBlocks = append(toolBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindCodeInterpreterResult,
				Payload: string(jsonResult),
			})
		default:
			toolBlocks = append(toolBlocks, types.MessageBlock{
				Kind:    types.MessageBlockKindNativeToolResult,
				Payload: string(jsonResult),
			})
		}
	}

	return tx.Message.Create().
		SetTaskID(taskID).
		SetSource(types.MessageSourceSystem).
		SetContent(&types.MessageContent{
			Blocks: toolBlocks,
		}).
		Save(ctx)
}

func calculateCost(usage model.Usage, model *memory.Model) float64 {
	return (float64(usage.InputTokens) * model.InputCost / 1000000) +
		(float64(usage.OutputTokens) * model.OutputCost / 1000000) +
		(float64(usage.CacheWriteTokens) * model.CacheWriteCost / 1000000) +
		(float64(usage.CacheReadTokens) * model.CacheReadCost / 1000000)
}

func logInterpreterArgs(ctx context.Context, taskID uuid.UUID, toolID string, args json.RawMessage) {
	var a codeact.InterpreterInput
	err := json.Unmarshal(args, &a)
	if err != nil {
		slog.ErrorContext(ctx, "failed to unmarshal interpreter args", "error", err)
		return
	}

	logInterpreter(ctx, taskID, toolID, a.Script, "args_interpreter")
}

func logInterpreterResult(ctx context.Context, taskID uuid.UUID, toolID string, result *codeact.InterpreterToolResult) {
	jsonResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal interpreter result", "error", err)
		return
	}

	logInterpreter(ctx, taskID, toolID, string(jsonResult), "result_interpreter")
}

func logInterpreter(ctx context.Context, taskID uuid.UUID, toolID string, content string, operation string) {
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

	type ToolOperation struct {
		ToolID  string `json:"tool_id"`
		Content string `json:"content"`
	}

	toolOperation := ToolOperation{
		ToolID:  toolID,
		Content: content,
	}

	jsonOperation, err := json.MarshalIndent(toolOperation, "", "  ")
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal tool operation", "error", err)
		return
	}

	_, err = fp.WriteString(string(jsonOperation) + "\n\n" + strings.Repeat("-", 100) + "\n\n")
	if err != nil {
		slog.ErrorContext(ctx, "failed to write interpreter args", "error", err)
	}
}

func (r *TaskReconciler) markMessageAsProcessed(ctx context.Context, message *memory.Message) error {
	_, err := message.Update().SetProcessedTime(time.Now()).Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to mark message as processed: %w", err)
	}
	return nil
}

func hasToolCalls(content []model.ContentBlock) bool {
	for _, block := range content {
		if _, ok := block.(*model.ToolCallBlock); ok {
			return true
		}
	}

	return false
}

func (r *TaskReconciler) publishMessage(taskID uuid.UUID, message *v1.Message) {
	event.Publish(r.bus, event.MessageEvent{
		MessageID: uuid.MustParse(message.Metadata.Id),
		TaskID:    taskID,
	})
}

func (r *TaskReconciler) publishDeltaMessage(taskID uuid.UUID, messageID uuid.UUID, content string) {
	event.Publish(r.bus, event.DeltaMessageEvent{
		MessageID: messageID,
		TaskID:    taskID,
		Content:   content,
	})
}

func (r *TaskReconciler) publishTaskEvent(taskID uuid.UUID) {
	event.Publish(r.bus, event.TaskEvent{
		TaskID: taskID,
	})
}

func (r *TaskReconciler) setTaskPhaseAndPublish(ctx context.Context, taskID uuid.UUID, phase TaskPhase) {
	p := convertTaskPhaseToMemory(phase)
	_, err := memory.Transaction(ctx, r.memory, func(tx *memory.Client) (*memory.Task, error) {
		task, err := tx.Task.UpdateOneID(taskID).SetPhase(p).Save(ctx)
		if err != nil {
			return nil, err
		}

		if phase == TaskPhaseAwaitInput && task.ParentTaskID != nil {
			err = r.rollupCostsToParent(ctx, tx, task)
			if err != nil {
				r.logger.ErrorContext(ctx, "failed to rollup costs to parent",
					"error", err,
					KeyTaskID, taskID,
					"parent_task_id", task.ParentTaskID,
				)
			}
		}

		return task, nil
	})

	if err != nil {
		r.logger.ErrorContext(ctx, "failed to set task phase",
			"error", err,
			KeyPhase, string(phase),
		)
	}

	r.logger.DebugContext(ctx, "task phase updated",
		KeyPhase, string(phase),
	)

	r.publishTaskEvent(taskID)
}

func (r *TaskReconciler) rollupCostsToParent(ctx context.Context, tx *memory.Client, subtask *memory.Task) error {
	if subtask.ParentTaskID == nil {
		return nil
	}

	parent, err := tx.Task.Get(ctx, *subtask.ParentTaskID)
	if err != nil {
		return fmt.Errorf("failed to get parent task: %w", err)
	}

	_, err = parent.Update().
		AddInputTokens(subtask.InputTokens).
		AddOutputTokens(subtask.OutputTokens).
		AddCacheWriteTokens(subtask.CacheWriteTokens).
		AddCacheReadTokens(subtask.CacheReadTokens).
		AddCost(subtask.Cost).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update parent task: %w", err)
	}

	r.logger.DebugContext(ctx, "rolled up subtask costs to parent",
		KeyTaskID, subtask.ID,
		"parent_task_id", parent.ID,
		"input_tokens", subtask.InputTokens,
		"output_tokens", subtask.OutputTokens,
		"cost", subtask.Cost,
	)

	return nil
}

func shouldGenerateTitle(task *memory.Task, messages []*memory.Message) bool {
	if task.Description != "" {
		return false
	}

	if !hasUserMessage(messages) {
		return false
	}

	return true
}

func (r *TaskReconciler) generateTitle(taskID uuid.UUID) {
	LogOperationStart(r.logger, "generate title")
	_, err, _ := r.titleGenGroup.Do(taskID.String(), func() (interface{}, error) {
		ctx := context.Background()
		generator := NewTitleGenerator(r.memory, r.providerFactory)
		return nil, generator.GenerateTitle(ctx, taskID)
	})
	LogOperationEnd(r.logger, "generate title", time.Now())

	if err != nil {
		LogError(r.logger, "failed to generate title", err)
		r.logger.ErrorContext(context.Background(), "failed to generate title", "error", err)
	}
}
