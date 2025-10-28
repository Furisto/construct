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
	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

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
		Concurrency: 50,
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

type Runtime struct {
	api            *api.Server
	memory         *memory.Client
	encryption     *secret.Encryption
	eventHub       *event.MessageHub
	bus            *event.Bus
	taskReconciler *TaskReconciler

	wg        sync.WaitGroup
	analytics analytics.Client
	metrics   *prometheus.Registry
}

func NewRuntime(memory *memory.Client, encryption *secret.Encryption, listener net.Listener, opts ...RuntimeOption) (*Runtime, error) {
	options := DefaultRuntimeOptions()
	for _, opt := range opts {
		opt(options)
	}

	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metricsRegistry.MustRegister(collectors.NewGoCollector())
	metricsRegistry.MustRegister(collectors.NewBuildInfoCollector())
	metricsRegistry.MustRegister(collectors.NewDBStatsCollector(memory.MustDB(), "construct"))

	messageHub, err := event.NewMessageHub(memory)
	if err != nil {
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
		metrics:        metricsRegistry,
	}

	api := api.NewServer(runtime, listener, runtime.bus, runtime.analytics)
	runtime.api = api

	return runtime, nil
}

func (rt *Runtime) Run(ctx context.Context) error {
	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		err := rt.api.ListenAndServe(ctx)
		if err != nil && err != http.ErrServerClosed {
			slog.Error("API server failed", "error", err)
		}
	}()

	rt.wg.Add(1)
	go func() {
		defer rt.wg.Done()
		rt.taskReconciler.Run(ctx)
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := rt.api.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error("failed to shutdown API server", "error", err)
	}

	stop := make(chan struct{})
	go func() {
		rt.wg.Wait()
		close(stop)
	}()

	select {
	case <-stop:
		return nil
	case <-shutdownCtx.Done():
		return fmt.Errorf("timed out while shutting down runtime")
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
