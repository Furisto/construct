package base

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql/schema"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/shared"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"

	_ "github.com/mattn/go-sqlite3"
)

type ToolTestSetup[ToolInput any, ToolResult any] struct {
	Call          func(ctx context.Context, services *ToolTestServices, input ToolInput) (ToolResult, error)
	QueryDatabase func(ctx context.Context, db *memory.Client) (any, error)
	CmpOptions    []cmp.Option
	Debug         bool
}

type ToolTestServices struct {
	DB *memory.Client
	FS afero.Fs
	CommandRunner shared.CommandRunner
}

type ToolTestScenario[ToolInput any, ToolResult any] struct {
	Name            string
	SeedDatabase    func(ctx context.Context, db *memory.Client)
	SeedFilesystem  func(ctx context.Context, fs afero.Fs)
	QueryFilesystem func(fs afero.Fs) (any, error)
	TestInput       ToolInput
	Expected        ToolTestExpectation[ToolResult]
}

type ToolTestExpectation[ToolResult any] struct {
	Database   any
	Filesystem any
	Result     ToolResult
	Error      error
}

func (s *ToolTestSetup[ToolInput, ToolResult]) RunToolTests(t *testing.T, scenarios []ToolTestScenario[ToolInput, ToolResult]) {
	if len(scenarios) == 0 {
		t.Fatalf("no scenarios provided")
	}

	if s.Call == nil {
		t.Fatalf("no call function provided")
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			db := s.SetupDatabase(t)
			defer db.Close()

			if scenario.SeedDatabase != nil {
				scenario.SeedDatabase(t.Context(), db)
			}

			fs := afero.NewMemMapFs()
			if scenario.SeedFilesystem != nil {
				scenario.SeedFilesystem(t.Context(), fs)
			}

			var actual ToolTestExpectation[ToolResult]
			output, err := s.Call(t.Context(), &ToolTestServices{DB: db, FS: fs}, scenario.TestInput)
			if err != nil {
				actual.Error = err
			} else {
				actual.Result = output
			}

			if s.QueryDatabase != nil && scenario.Expected.Database != nil {
				resources, err := s.QueryDatabase(t.Context(), db)
				if err != nil {
					t.Fatalf("failed to query database resources: %v", err)
				}
				actual.Database = resources
			}

			if scenario.QueryFilesystem != nil {
				filesystem, err := scenario.QueryFilesystem(fs)
				if err != nil {
					t.Fatalf("failed to query filesystem: %v", err)
				}
				actual.Filesystem = filesystem
			}

			if diff := cmp.Diff(scenario.Expected, actual, s.CmpOptions...); diff != "" {
				if s.Debug {
					s.DebugDatabase(t, db)
				}
				t.Errorf("%s() mismatch (-want +got):\n%s", scenario.Name, diff)
			}
		})
	}
}

func (s *ToolTestSetup[ToolInput, ToolOutput]) SetupDatabase(t *testing.T) *memory.Client {
	t.Helper()

	db, err := memory.Open(dialect.SQLite, "file:construct_test?mode=memory&cache=private&_fk=1")
	if err != nil {
		t.Fatalf("failed opening connection to sqlite: %v", err)
	}

	err = db.Schema.Create(t.Context())
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	if s.Debug {
		s.DebugSchema(t.Context(), t, db)
	}

	return db
}

func (s *ToolTestSetup[ToolInput, ToolOutput]) DebugSchema(ctx context.Context, t *testing.T, db *memory.Client) {
	t.Helper()

	tempFile, err := os.CreateTemp("", "tool_test_schema_*.sql")
	if err != nil {
		t.Fatalf("failed creating schema file: %v", err)
	}

	err = db.Schema.WriteTo(ctx, tempFile, schema.WithIndent(" "))
	if err != nil {
		t.Fatalf("failed writing schema to file: %v", err)
	}

	t.Logf("schema: %v", tempFile.Name())
}

func (s *ToolTestSetup[ToolInput, ToolOutput]) DebugDatabase(t *testing.T, db *memory.Client) {
	t.Helper()

	modelProviders, err := db.ModelProvider.Query().All(t.Context())
	if err != nil {
		t.Fatalf("failed querying model providers: %v", err)
	}

	models, err := db.Model.Query().All(t.Context())
	if err != nil {
		t.Fatalf("failed querying models: %v", err)
	}

	agents, err := db.Agent.Query().All(t.Context())
	if err != nil {
		t.Fatalf("failed querying agents: %v", err)
	}

	tasks, err := db.Task.Query().All(t.Context())
	if err != nil {
		t.Fatalf("failed querying tasks: %v", err)
	}

	messages, err := db.Message.Query().All(t.Context())
	if err != nil {
		t.Fatalf("failed querying messages: %v", err)
	}

	tempFile, err := os.CreateTemp("", "database*.json")
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
