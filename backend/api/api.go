package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/analytics"
	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	"github.com/google/uuid"
)

type AgentRuntime interface {
	Memory() *memory.Client
	Encryption() *secret.Client
	EventHub() *event.MessageHub
	CancelTask(id uuid.UUID)
}

type Server struct {
	mux      *http.ServeMux
	server   *http.Server
	listener net.Listener
}

func NewServer(runtime AgentRuntime, listener net.Listener, eventBus *event.Bus, analyticsClient analytics.Client) *Server {
	apiHandler := NewHandler(
		HandlerOptions{
			DB:           runtime.Memory(),
			Encryption:   runtime.Encryption(),
			AgentRuntime: runtime,
			MessageHub:   runtime.EventHub(),
			EventBus:     eventBus,
			Analytics:    analyticsClient,
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", apiHandler))
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{\"status\":\"ok\"}"))
	}))

	return &Server{
		mux:      mux,
		listener: listener,
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	s.server = &http.Server{
		Handler: s.mux,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	return s.server.Serve(s.listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

type HandlerOptions struct {
	DB           *memory.Client
	Encryption   *secret.Client
	AgentRuntime AgentRuntime

	EventBus   *event.Bus
	MessageHub *event.MessageHub
	Analytics  analytics.Client

	RequestOptions []connect.HandlerOption
}

type Handler struct {
	mux *http.ServeMux
}

func NewHandler(opts HandlerOptions) *Handler {
	handler := &Handler{
		mux: http.NewServeMux(),
	}

	modelProviderHandler := NewModelProviderHandler(opts.DB, opts.Encryption)
	handler.mux.Handle(v1connect.NewModelProviderServiceHandler(modelProviderHandler, opts.RequestOptions...))

	modelHandler := NewModelHandler(opts.DB)
	handler.mux.Handle(v1connect.NewModelServiceHandler(modelHandler, opts.RequestOptions...))

	agentHandler := NewAgentHandler(opts.DB, opts.Analytics)
	handler.mux.Handle(v1connect.NewAgentServiceHandler(agentHandler, opts.RequestOptions...))

	taskHandler := NewTaskHandler(opts.DB, opts.MessageHub, opts.EventBus, opts.AgentRuntime, opts.Analytics)
	handler.mux.Handle(v1connect.NewTaskServiceHandler(taskHandler, opts.RequestOptions...))

	messageHandler := NewMessageHandler(opts.DB, opts.AgentRuntime, opts.MessageHub, opts.EventBus)
	handler.mux.Handle(v1connect.NewMessageServiceHandler(messageHandler, opts.RequestOptions...))

	return handler
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func apiError(err error) error {
	if connect.CodeOf(err) != connect.CodeUnknown {
		return err
	}

	if memory.IsNotFound(err) {
		return connect.NewError(connect.CodeNotFound, sanitizeError(err))
	}

	return connect.NewError(connect.CodeInternal, sanitizeError(err))
}

func sanitizeError(err error) error {
	return errors.New(strings.ReplaceAll(err.Error(), "memory: ", ""))
}
