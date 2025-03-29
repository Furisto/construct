package api

import (
	"context"
	"testing"

	"net/http/httptest"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect"
	"github.com/furisto/construct/api/go/client"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/secret"
	"github.com/google/go-cmp/cmp"
)

type ClientServiceCall[Request any, Response any] func(ctx context.Context, client *client.Client, req *connect.Request[Request]) (*connect.Response[Response], error)

type ServiceTestSetup[Request any, Response any] struct {
	Call       ClientServiceCall[Request, Response]
	CmpOptions []cmp.Option
}

type ServiceTestExpectation[Response any] struct {
	Response Response
	Error    string
	Database []any
}

type ServiceTestScenario[Request any, Response any] struct {
	Name         string
	SeedDatabase func(ctx context.Context, db *memory.Client) error
	Request      *Request
	Expected     ServiceTestExpectation[Response]
}

func (s *ServiceTestSetup[Request, Response]) RunServiceTests(t *testing.T, scenarios []ServiceTestScenario[Request, Response]) {
	ctx := context.Background()
	handlerOptions := DefaultTestHandlerOptions(t)
	server := NewTestServer(ctx, t, handlerOptions)

	server.Start()
	defer server.Close()

	client, err := client.NewClient(ctx, server.API.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			server.ClearDatabase()
			if scenario.SeedDatabase != nil {
				if err := scenario.SeedDatabase(ctx, server.Options.DB); err != nil {
					t.Fatalf("failed to seed database: %v", err)
				}
			}

			actual := ServiceTestExpectation[Response]{}
			response, err := s.Call(ctx, client, connect.NewRequest(scenario.Request))

			if err != nil {
				actual.Error = err.Error()
			}

			if response != nil {
				actual.Response = *response.Msg
			}

			if diff := cmp.Diff(scenario.Expected, actual, s.CmpOptions...); diff != "" {
				t.Errorf("%s() mismatch (-want +got):\n%s", scenario.Name, diff)
			}
		})
	}
}

func DefaultTestHandlerOptions(t *testing.T) HandlerOptions {
	db, err := memory.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
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

func (s *TestServer) ClearDatabase() {
	s.Options.DB.ModelProvider.Delete().Exec(context.Background())
}
