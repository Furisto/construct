package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/stream"
	"github.com/google/uuid"
)

type AgentRuntime interface {
	Memory() *memory.Client
	Encryption() *secret.Client
	TriggerReconciliation(id uuid.UUID)
	EventHub() *stream.EventHub
}

type Server struct {
	mux      *http.ServeMux
	server   *http.Server
	listener net.Listener
}

func NewServer(runtime AgentRuntime, listener net.Listener) *Server {
	apiHandler := NewHandler(
		HandlerOptions{
			DB:           runtime.Memory(),
			Encryption:   runtime.Encryption(),
			AgentRuntime: runtime,
			MessageHub:   runtime.EventHub(),
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", apiHandler))

	return &Server{
		mux:      mux,
		listener: listener,
	}
}

func (s *Server) ListenAndServe() error {
	s.server = &http.Server{
		Handler: s.mux,
	}

	return s.server.Serve(s.listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

type HandlerOptions struct {
	DB             *memory.Client
	Encryption     *secret.Client
	RequestOptions []connect.HandlerOption
	AgentRuntime   AgentRuntime
	MessageHub     *stream.EventHub
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

	agentHandler := NewAgentHandler(opts.DB)
	handler.mux.Handle(v1connect.NewAgentServiceHandler(agentHandler, opts.RequestOptions...))

	taskHandler := NewTaskHandler(opts.DB)
	handler.mux.Handle(v1connect.NewTaskServiceHandler(taskHandler, opts.RequestOptions...))

	messageHandler := NewMessageHandler(opts.DB, opts.AgentRuntime, opts.MessageHub)
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
