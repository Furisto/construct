package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"net/http/httptest"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	api_client "github.com/furisto/construct/api/go/client"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	"github.com/google/go-cmp/cmp"
)

type ClientServiceCall[Request any, Response any] func(ctx context.Context, client *api_client.Client, req *connect.Request[Request]) (*connect.Response[Response], error)

type ServiceTestSetup[Request any, Response any] struct {
	Call       ClientServiceCall[Request, Response]
	CmpOptions []cmp.Option
	Debug      bool
}

type ServiceTestExpectation[Response any] struct {
	Response Response
	Error    string
	Database []any
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
	server := NewTestServer(ctx, t, handlerOptions)

	server.Start()
	defer server.Close()

	apiClient, err := api_client.NewClient(ctx, server.API.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

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

			if diff := cmp.Diff(scenario.Expected, actual, s.CmpOptions...); diff != "" {
				server.DebugDatabase(ctx, t)
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

	return HandlerOptions{
		DB:         db,
		Encryption: encryption,
	}
}

type TestServer struct {
	API     *httptest.Server
	Options HandlerOptions
}

func NewTestServer(ctx context.Context, t *testing.T, handlerOptions HandlerOptions) *TestServer {
	server := httptest.NewUnstartedServer(NewHandler(handlerOptions))

	if err := handlerOptions.DB.Schema.Create(ctx); err != nil {
		t.Fatalf("failed creating schema resources: %v", err)
	}

	return &TestServer{
		API:     server,
		Options: handlerOptions,
	}
}

func (s *TestServer) Start() {
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

func strPtr(s string) *string {
	return &s
}
