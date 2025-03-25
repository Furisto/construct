package api

import (
	"net/http"

	"github.com/furisto/construct/backend/agent"
)

type ApiHandler struct {
	agent *agent.Agent
}

func NewApiHandler(agent *agent.Agent) *ApiHandler {
	return &ApiHandler{agent: agent}
}

func (h *ApiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	
}