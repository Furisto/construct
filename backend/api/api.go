package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/api/socket"
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
	mux              *http.ServeMux
	server           *http.Server
	options          ServerOptions
	listenerProvider socket.ListenerProvider
}

type ConnectionOption string

const (
	ConnectionOptionTCP  ConnectionOption = "tcp"
	ConnectionOptionUnix ConnectionOption = "unix"
)

type ServerOptions struct {
	Port       int
	Connection ConnectionOption
	SocketPath string
}

type ServerOption func(*ServerOptions)

func WithPort(port int) ServerOption {
	return func(opts *ServerOptions) {
		opts.Port = port
	}
}

func WithConnection(conn ConnectionOption) ServerOption {
	return func(opts *ServerOptions) {
		opts.Connection = conn
	}
}

func WithSocketPath(path string) ServerOption {
	return func(opts *ServerOptions) {
		opts.SocketPath = path
	}
}

func NewServer(runtime AgentRuntime, opts ...ServerOption) *Server {
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

	serverOptions := ServerOptions{
		Port:       29333,
		Connection: ConnectionOptionTCP,
		SocketPath: "",
	}

	for _, opt := range opts {
		opt(&serverOptions)
	}

	return &Server{
		mux:     mux,
		options: serverOptions,
	}
}

func (s *Server) ListenAndServe() error {
	listener, err := s.listenerProvider.Listener()
	if err != nil {
		return fmt.Errorf("failed to listen via listener provider: %w", err)
	}

	s.server = &http.Server{
		Handler: s.mux,
	}

	return s.server.Serve(listener)

	// // Fallback to TCP
	// s.server = &http.Server{
	// 	Addr:    fmt.Sprintf(":%d", s.options.Port),
	// 	Handler: s.mux,
	// }

	// return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.listenerProvider != nil {
		defer s.listenerProvider.Close()
	}
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
