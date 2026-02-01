package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"strings"

	"connectrpc.com/connect"

	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/analytics"
	"github.com/furisto/construct/backend/api/auth"
	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/skill"
)

type AgentRuntime interface {
	Memory() *memory.Client
	Encryption() *secret.Encryption
}

type Server struct {
	mux      *http.ServeMux
	server   *http.Server
	listener net.Listener
}

func NewServer(runtime AgentRuntime, listener net.Listener, eventBus *event.Bus, analyticsClient analytics.Client, skillInstaller *skill.SkillManager) *Server {
	tokenProvider := auth.NewTokenProvider()

	apiHandler := NewHandler(
		HandlerOptions{
			DB:            runtime.Memory(),
			Encryption:    runtime.Encryption(),
			AgentRuntime:  runtime,
			EventBus:      eventBus,
			Analytics:     analyticsClient,
			TokenProvider: tokenProvider,
			Skills:        skillInstaller,
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
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			transport := auth.TransportTCP
			if _, ok := c.(*net.UnixConn); ok {
				transport = auth.TransportUnix
			}
			return auth.WithTransport(ctx, transport)
		},
	}

	return s.server.Serve(s.listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

type HandlerOptions struct {
	DB            *memory.Client
	Encryption    *secret.Encryption
	AgentRuntime  AgentRuntime
	TokenProvider *auth.TokenProvider
	Skills        *skill.SkillManager

	EventBus  *event.Bus
	Analytics analytics.Client

	RequestOptions []connect.HandlerOption
}

type Handler struct {
	mux *http.ServeMux
}

func NewHandler(opts HandlerOptions) *Handler {
	handler := &Handler{
		mux: http.NewServeMux(),
	}

	connectOpts := append([]connect.HandlerOption{connect.WithInterceptors(auth.NewAuthInterceptor(opts.DB, opts.TokenProvider))}, opts.RequestOptions...)

	authHandler := NewAuthHandler(opts.DB, opts.TokenProvider)
	handler.mux.Handle(v1connect.NewAuthServiceHandler(authHandler, connectOpts...))

	modelProviderHandler := NewModelProviderHandler(opts.DB, opts.Encryption)
	handler.mux.Handle(v1connect.NewModelProviderServiceHandler(modelProviderHandler, connectOpts...))

	modelHandler := NewModelHandler(opts.DB)
	handler.mux.Handle(v1connect.NewModelServiceHandler(modelHandler, connectOpts...))

	agentHandler := NewAgentHandler(opts.DB, opts.Analytics)
	handler.mux.Handle(v1connect.NewAgentServiceHandler(agentHandler, connectOpts...))

	taskHandler := NewTaskHandler(opts.DB, opts.EventBus, opts.AgentRuntime, opts.Analytics)
	handler.mux.Handle(v1connect.NewTaskServiceHandler(taskHandler, connectOpts...))

	messageHandler := NewMessageHandler(opts.DB, opts.AgentRuntime, opts.EventBus)
	handler.mux.Handle(v1connect.NewMessageServiceHandler(messageHandler, connectOpts...))

	skillHandler := NewSkillHandler(opts.Skills)
	handler.mux.Handle(v1connect.NewSkillServiceHandler(skillHandler, connectOpts...))

	return handler
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func apiError(err error) error {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		return err
	}
	slog.Error("error in api handler", "error", err, "caller_file", file, "caller_line", line)

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
