package api

import (
	"net/http"

	"connectrpc.com/connect"
	"github.com/furisto/construct/backend/memory"
)

type HandlerOptions struct {
	DB *memory.Client
}

type Handler struct {
	db *memory.Client
}

func NewHandler(opts HandlerOptions) *Handler {
	handler := &Handler{
		db: opts.DB,
	}

	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	
}

func apiError(error error) *connect.Error {
	return connect.NewError(connect.CodeInternal, error)
}
