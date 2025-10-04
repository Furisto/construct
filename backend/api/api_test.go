package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	api_client "github.com/furisto/construct/api/go/client"
	"github.com/furisto/construct/backend/analytics"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	"github.com/furisto/construct/backend/stream"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

type ClientServiceCall[Request any, Response any] func(ctx context.Context, client *api_client.Client, req *connect.Request[Request]) (*connect.Response[Response], error)

type ServiceTestSetup[Request any, Response any] struct {
	Call          ClientServiceCall[Request, Response]
	CmpOptions    []cmp.Option
	Debug         bool
	QueryDatabase func(ctx context.Context, db *memory.Client) (any, error)
}

type ServiceTestExpectation[Response any] struct {
	Response  Response
	Error     string
	Database  any
	Analytics []analytics.Event
}

type ServiceTestScenario[Request any, Response any] struct {
	Name         string
	SeedDatabase func(ctx context.Context, db *memory.Client)
	Request      *Request
	Expected     ServiceTestExpectation[Response]
}

func (s *ServiceTestSetup[Request, Response]) RunServiceTests(t *testing.T, scenarios []ServiceTestScenario[Request, Response]) {
	if len(scenarios) == 0 {
		t.Fatalf("no scenarios provided")
	}

	if s.Call == nil {
		t.Fatalf("no call function provided")
	}

	ctx := context.Background()
	handlerOptions := DefaultTestHandlerOptions(t)
	server := NewTestServer(t, handlerOptions)

	server.Start(ctx)
	defer server.Close()

	apiClient := api_client.NewClient(api_client.EndpointContext{
		Address: server.API.URL,
		Kind:    "http",
	})

	if s.Debug {
		server.DebugSchema(ctx, t)
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			err := server.ClearDatabase(ctx, t)
			if err != nil {
				t.Fatalf("failed to clear database: %v", sanitizeError(err))
			}

			if scenario.SeedDatabase != nil {
				scenario.SeedDatabase(ctx, server.Options.DB)
			}

			actual := ServiceTestExpectation[Response]{}
			response, err := s.Call(ctx, apiClient, connect.NewRequest(scenario.Request))

			if err != nil {
				actual.Error = err.Error()
			}

			if response != nil {
				actual.Response = *response.Msg
			}

			if len(scenario.Expected.Analytics) > 0 {
				actual.Analytics = server.Options.Analytics.(*analytics.InMemoryClient).Events
			}

			if s.QueryDatabase != nil && scenario.Expected.Database != nil {
				resources, err := s.QueryDatabase(ctx, server.Options.DB)
				if err != nil {
					t.Fatalf("failed to query database resources: %v", err)
				}
				actual.Database = resources
			}

			if diff := cmp.Diff(scenario.Expected, actual, s.CmpOptions...); diff != "" {
				if s.Debug {
					server.DebugDatabase(ctx, t)
				}
				t.Errorf("%s() mismatch (-want +got):\n%s", scenario.Name, diff)
			}
		})
	}
}

func DefaultTestHandlerOptions(t *testing.T) HandlerOptions {
	db, err := memory.Open(dialect.SQLite, "file:construct_test?mode=memory&cache=private&_fk=1")
	if err != nil {
		t.Fatalf("failed opening connection to sqlite: %v", err)
	}

	keyset, err := secret.GenerateKeyset()
	if err != nil {
		t.Fatalf("failed generating keyset: %v", err)
	}

	encryption, err := secret.NewClient(keyset)
	if err != nil {
		t.Fatalf("failed creating encryption client: %v", err)
	}

	runtime := &MockAgentRuntime{}

	return HandlerOptions{
		DB:           db,
		Encryption:   encryption,
		AgentRuntime: runtime,
		Analytics:    analytics.NewInMemoryClient(),
	}
}

type TestServer struct {
	API     *httptest.Server
	Options HandlerOptions

	t *testing.T
}

func NewTestServer(t *testing.T, handlerOptions HandlerOptions) *TestServer {
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", NewHandler(handlerOptions)))
	server := httptest.NewUnstartedServer(mux)

	return &TestServer{
		API:     server,
		Options: handlerOptions,
		t:       t,
	}
}

func (s *TestServer) Start(ctx context.Context) {
	if err := s.Options.DB.Schema.Create(ctx); err != nil {
		s.t.Fatalf("failed creating schema resources: %v", err)
	}
	s.API.Start()
}

func (s *TestServer) Close() {
	s.API.Close()
}

func (s *TestServer) ClearDatabase(ctx context.Context, t *testing.T) error {
	t.Helper()

	_, err := memory.Transaction(ctx, s.Options.DB, func(tx *memory.Client) (*any, error) {
		_, err := tx.Message.Delete().Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to delete messages: %w", err)
		}

		_, err = tx.Task.Delete().Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to delete tasks: %w", err)
		}

		_, err = tx.Agent.Delete().Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to delete agents: %w", err)
		}

		_, err = tx.Model.Delete().Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to delete models: %w", err)
		}

		_, err = tx.ModelProvider.Delete().Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to delete model providers: %w", err)
		}

		return nil, nil
	})

	return err
}

func (s *TestServer) DebugSchema(ctx context.Context, t *testing.T) {
	t.Helper()

	tempFile, err := os.CreateTemp("", "schema.sql")
	if err != nil {
		t.Fatalf("failed creating schema file: %v", err)
	}

	err = s.Options.DB.Schema.WriteTo(ctx, tempFile, schema.WithIndent(" "))
	if err != nil {
		t.Fatalf("failed writing schema to file: %v", err)
	}

	t.Logf("schema: %v", tempFile.Name())
}

func (s *TestServer) DebugDatabase(ctx context.Context, t *testing.T) {
	t.Helper()

	modelProviders, err := s.Options.DB.ModelProvider.Query().All(ctx)
	if err != nil {
		t.Fatalf("failed querying model providers: %v", err)
	}

	models, err := s.Options.DB.Model.Query().All(ctx)
	if err != nil {
		t.Fatalf("failed querying models: %v", err)
	}

	agents, err := s.Options.DB.Agent.Query().All(ctx)
	if err != nil {
		t.Fatalf("failed querying agents: %v", err)
	}

	tasks, err := s.Options.DB.Task.Query().All(ctx)
	if err != nil {
		t.Fatalf("failed querying tasks: %v", err)
	}

	messages, err := s.Options.DB.Message.Query().All(ctx)
	if err != nil {
		t.Fatalf("failed querying messages: %v", err)
	}

	tempFile, err := os.CreateTemp("", "database.json")
	if err != nil {
		t.Fatalf("failed creating temp file: %v", err)
	}

	err = json.NewEncoder(tempFile).Encode(map[string]any{
		"modelProviders": modelProviders,
		"models":         models,
		"agents":         agents,
		"tasks":          tasks,
		"messages":       messages,
	})
	if err != nil {
		t.Fatalf("failed encoding database: %v", err)
	}

	t.Logf("database: %v", tempFile.Name())
}

func ptr[T any](v T) *T {
	return &v
}

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

type MockAgentRuntime struct {
}

func (m *MockAgentRuntime) Memory() *memory.Client {
	return nil
}

func (m *MockAgentRuntime) Encryption() *secret.Client {
	return nil
}

func (m *MockAgentRuntime) EventHub() *stream.EventHub {
	return nil
}

func (m *MockAgentRuntime) TriggerReconciliation(id uuid.UUID) {
}

func (m *MockAgentRuntime) CancelTask(id uuid.UUID) {
}
