package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	"github.com/google/uuid"
)

type AgentRuntime interface {
	GetMemory() *memory.Client
	GetEncryption() *secret.Client
	TriggerReconciliation(id uuid.UUID)
}

type Server struct {
	mux    *http.ServeMux
	server *http.Server
	port   int
}

func NewServer(runtime AgentRuntime, port int) *Server {
	apiHandler := NewHandler(
		HandlerOptions{
			DB:           runtime.GetMemory(),
			Encryption:   runtime.GetEncryption(),
			AgentRuntime: runtime,
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)

	return &Server{
		mux:  http.NewServeMux(),
		port: port,
	}
}

func (s *Server) ListenAndServe() error {
	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.mux,
	}

	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

type HandlerOptions struct {
	DB             *memory.Client
	Encryption     *secret.Client
	RequestOptions []connect.HandlerOption
	AgentRuntime   AgentRuntime
}

type Handler struct {
	db         *memory.Client
	encryption *secret.Client
	mux        *http.ServeMux
}

func NewHandler(opts HandlerOptions) *Handler {
	handler := &Handler{
		db:         opts.DB,
		encryption: opts.Encryption,
		mux:        http.NewServeMux(),
	}

	modelProviderHandler := NewModelProviderHandler(handler.db, handler.encryption)
	handler.mux.Handle(v1connect.NewModelProviderServiceHandler(modelProviderHandler, opts.RequestOptions...))

	modelHandler := NewModelHandler(opts.DB)
	handler.mux.Handle(v1connect.NewModelServiceHandler(modelHandler, opts.RequestOptions...))

	agentHandler := NewAgentHandler(opts.DB)
	handler.mux.Handle(v1connect.NewAgentServiceHandler(agentHandler, opts.RequestOptions...))

	taskHandler := NewTaskHandler(handler.db)
	handler.mux.Handle(v1connect.NewTaskServiceHandler(taskHandler, opts.RequestOptions...))

	messageHandler := NewMessageHandler(handler.db)
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


