package api

import (
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
)

type HandlerOptions struct {
	DB         *memory.Client
	Encryption *secret.Client
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
	handler.mux.Handle(v1connect.NewModelProviderServiceHandler(modelProviderHandler))

	agentHandler := NewAgentHandler(handler.db)
	handler.mux.Handle(v1connect.NewAgentServiceHandler(agentHandler))

	taskHandler := NewTaskHandler(handler.db)
	handler.mux.Handle(v1connect.NewTaskServiceHandler(taskHandler))

	messageHandler := NewMessageHandler(handler.db)
	handler.mux.Handle(v1connect.NewMessageServiceHandler(messageHandler))

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
