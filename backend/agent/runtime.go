package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/analytics"
	"github.com/furisto/construct/backend/api"
	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/skill"
	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/furisto/construct/shared"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const DefaultServerPort = 29333

type RuntimeOptions struct {
	Tools        []codeact.Tool
	Concurrency  int
	Analytics    analytics.Client
	LoggerConfig *LoggerConfig
}

func DefaultRuntimeOptions() *RuntimeOptions {
	return &RuntimeOptions{
		Tools:        []codeact.Tool{},
		Concurrency:  50,
		LoggerConfig: DefaultLoggerConfig(),
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

func WithLoggerConfig(config *LoggerConfig) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.LoggerConfig = config
	}
}

type Runtime struct {
	api            *api.Server
	memory         *memory.Client
	encryption     *secret.Encryption
	eventHub       *event.MessageHub
	bus            *event.Bus
	taskReconciler *TaskReconciler
	logger         *slog.Logger

	wg        sync.WaitGroup
	analytics analytics.Client
	metrics   *prometheus.Registry
}

func NewRuntime(memory *memory.Client, encryption *secret.Encryption, listener net.Listener, opts ...RuntimeOption) (*Runtime, error) {
	options := DefaultRuntimeOptions()
	for _, opt := range opts {
		opt(options)
	}

	logger := slog.With(
		KeyComponent, "agent_runtime",
	)

	LogComponentStartup(logger, "agent_runtime",
		KeyConcurrency, options.Concurrency,
		KeyToolsRegistered, len(options.Tools),
	)

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsRegistry.MustRegister(collectors.NewGoCollector())
	metricsRegistry.MustRegister(collectors.NewBuildInfoCollector())
	metricsRegistry.MustRegister(collectors.NewDBStatsCollector(memory.MustDB(), "construct"))

	messageHub, err := event.NewMessageHub(memory)
	if err != nil {
		LogError(logger, "initialize message hub", err)
		return nil, err
	}
	eventBus := event.NewBus(metricsRegistry)

	interceptors := []codeact.Interceptor{
		codeact.InterceptorFunc(codeact.ToolStatisticsInterceptor),
		codeact.InterceptorFunc(codeact.DurableFunctionInterceptor),
		codeact.NewToolEventPublisher(messageHub),
		codeact.InterceptorFunc(codeact.ResetTemporarySessionValuesInterceptor),
	}

	clientFactory := NewModelProviderFactory(encryption, memory)

	runtime := &Runtime{
		memory:         memory,
		encryption:     encryption,
		eventHub:       messageHub,
		bus:            eventBus,
		taskReconciler: NewTaskReconciler(memory, codeact.NewInterpreter(options.Tools, interceptors), options.Concurrency, eventBus, messageHub, clientFactory, metricsRegistry),
		analytics:      options.Analytics,
		logger:         logger,
		metrics:        metricsRegistry,
	}

	fs := afero.NewOsFs()
	userInfo := shared.NewDefaultUserInfo(&afero.Afero{Fs: fs})
	skills := skill.NewSkillManager(fs, userInfo)

	api := api.NewServer(runtime, listener, runtime.bus, runtime.analytics, skills)
	runtime.api = api

	listenerAddr := listener.Addr().String()
	listenerType := listener.Addr().Network()
	runtime.logger.Info("API server configured",
		KeyListenerAddress, listenerAddr,
		KeyListenerType, listenerType,
	)

	return runtime, nil
}

func (rt *Runtime) Run(ctx context.Context) error {
	rt.logger.Info("agent runtime starting")

	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		LogComponentStartup(rt.logger, "API server")
		err := rt.api.ListenAndServe(ctx)
		if err != nil && err != http.ErrServerClosed {
			LogError(rt.logger, "API server listen and serve", err)
		}
	}()

	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		LogComponentStartup(rt.logger, "task reconciler")
		err := rt.taskReconciler.Run(ctx)
		if err != nil {
			LogError(rt.logger, "task reconciler run", err)
		}
	}()

	rt.logger.Info("agent runtime fully initialized, waiting for shutdown signal")
	<-ctx.Done()

	rt.logger.Info("agent runtime shutdown initiated")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownStart := time.Now()
	rt.logger.Debug("shutting down API server")
	err := rt.api.Shutdown(shutdownCtx)
	if err != nil {
		LogError(rt.logger, "API server shutdown", err)
	}
	LogComponentShutdown(rt.logger, "API server", shutdownStart)

	stop := make(chan struct{})
	go func() {
		rt.wg.Wait()
		close(stop)
	}()

	select {
	case <-stop:
		rt.logger.Info("agent runtime shutdown complete")
		return nil
	case <-shutdownCtx.Done():
		rt.logger.Error("shutdown timeout", "error", fmt.Errorf("timed out while shutting down runtime"))
		return err
	}
}

func (rt *Runtime) Encryption() *secret.Encryption {
	return rt.encryption
}

func (rt *Runtime) Memory() *memory.Client {
	return rt.memory
}

func (rt *Runtime) EventHub() *event.MessageHub {
	return rt.eventHub
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
