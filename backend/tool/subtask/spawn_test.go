package subtask

import (
	"context"
	"testing"

	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/message"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/memory/task"
	"github.com/furisto/construct/backend/memory/test"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	_ "modernc.org/sqlite"
)

func TestSpawnTask(t *testing.T) {
	t.Parallel()

	type SubtaskResult struct {
		ID             uuid.UUID
		AgentID        uuid.UUID
		ParentID       uuid.UUID
		DesiredPhase   types.TaskPhase
		ProjectDir     string
		InitialMessage string
		MessageSource  types.MessageSource
	}

	parentTaskID := uuid.New()
	parentAgentID := uuid.New()
	targetAgentID := uuid.New()

	setup := &base.ToolTestSetup[*SpawnTaskInput, *SpawnTaskResult]{
		Call: func(ctx context.Context, services *base.ToolTestServices, input *SpawnTaskInput) (*SpawnTaskResult, error) {
			bus := event.NewBus(nil)
			return SpawnTask(ctx, services.DB, bus, parentTaskID, input)
		},
		QueryDatabase: func(ctx context.Context, db *memory.Client) (any, error) {
			var results []SubtaskResult

			subtasks, err := db.Task.Query().Where(task.HasParentWith(task.IDEQ(parentTaskID))).All(ctx)
			if err != nil {
				return nil, err
			}

			for _, subtask := range subtasks {
				sr := SubtaskResult{
					ID:           subtask.ID,
					AgentID:      subtask.AgentID,
					ParentID:     *subtask.ParentTaskID,
					DesiredPhase: subtask.DesiredPhase,
					ProjectDir:   subtask.ProjectDirectory,
				}

				msg, err := db.Message.Query().Where(message.TaskIDEQ(subtask.ID)).First(ctx)
				if err == nil && msg != nil && msg.Content != nil && len(msg.Content.Blocks) > 0 {
					sr.InitialMessage = msg.Content.Blocks[0].Payload
					sr.MessageSource = msg.Source
				}

				results = append(results, sr)
			}

			return results, nil
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreFields(base.ToolError{}, "Suggestions"),
			cmpopts.IgnoreFields(SubtaskResult{}, "ID"),
		},
	}

	setup.RunToolTests(t, []base.ToolTestScenario[*SpawnTaskInput, *SpawnTaskResult]{
		{
			Name: "successful subtask creation",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				parentAgent := test.NewAgentBuilder(t, parentAgentID, db, model).
					WithName("parent").
					Build(ctx)

				test.NewAgentBuilder(t, targetAgentID, db, model).
					WithName("reviewer").
					Build(ctx)

				test.NewTaskBuilder(t, parentTaskID, db, parentAgent).Build(ctx)
			},
			TestInput: &SpawnTaskInput{
				Agent:  "reviewer",
				Prompt: "Review src/auth.go for security issues",
			},
			Expected: base.ToolTestExpectation[*SpawnTaskResult]{
				Database: []SubtaskResult{
					{
						AgentID:        targetAgentID,
						ParentID:       parentTaskID,
						DesiredPhase:   types.TaskPhaseSuspended,
						ProjectDir:     "",
						InitialMessage: "Review src/auth.go for security issues",
						MessageSource:  types.MessageSourceUser,
					},
				},
			},
		},
		{
			Name: "successful subtask creation with project directory",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				parentAgent := test.NewAgentBuilder(t, parentAgentID, db, model).
					WithName("parent").
					Build(ctx)

				test.NewAgentBuilder(t, targetAgentID, db, model).
					WithName("coder").
					Build(ctx)

				parentTask, err := db.Task.Create().
					SetID(parentTaskID).
					SetAgentID(parentAgent.ID).
					SetProjectDirectory("/workspace/project").
					Save(ctx)
				if err != nil {
					t.Fatalf("failed to create parent task: %v", err)
				}
				_ = parentTask
			},
			TestInput: &SpawnTaskInput{
				Agent:  "coder",
				Prompt: "Implement authentication module",
			},
			Expected: base.ToolTestExpectation[*SpawnTaskResult]{
				Database: []SubtaskResult{
					{
						AgentID:        targetAgentID,
						ParentID:       parentTaskID,
						DesiredPhase:   types.TaskPhaseSuspended,
						ProjectDir:     "/workspace/project",
						InitialMessage: "Implement authentication module",
						MessageSource:  types.MessageSourceUser,
					},
				},
			},
		},
		{
			Name: "agent does not exist",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				parentAgent := test.NewAgentBuilder(t, parentAgentID, db, model).
					WithName("parent").
					Build(ctx)

				test.NewTaskBuilder(t, parentTaskID, db, parentAgent).Build(ctx)
			},
			TestInput: &SpawnTaskInput{
				Agent:  "nonexistent",
				Prompt: "Do something",
			},
			Expected: base.ToolTestExpectation[*SpawnTaskResult]{
				Error: base.NewCustomError("agent with name nonexistent does not exist", []string{}),
			},
		},
		{
			Name: "empty agent name",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				parentAgent := test.NewAgentBuilder(t, parentAgentID, db, model).
					WithName("parent").
					Build(ctx)

				test.NewTaskBuilder(t, parentTaskID, db, parentAgent).Build(ctx)
			},
			TestInput: &SpawnTaskInput{
				Agent:  "",
				Prompt: "Do something",
			},
			Expected: base.ToolTestExpectation[*SpawnTaskResult]{
				Error: base.NewCustomError("agent is required", []string{
					"Provide the name or ID of the agent to assign to the subtask",
				}),
			},
		},
		{
			Name: "empty prompt",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				parentAgent := test.NewAgentBuilder(t, parentAgentID, db, model).
					WithName("parent").
					Build(ctx)

				test.NewAgentBuilder(t, targetAgentID, db, model).
					WithName("reviewer").
					Build(ctx)

				test.NewTaskBuilder(t, parentTaskID, db, parentAgent).Build(ctx)
			},
			TestInput: &SpawnTaskInput{
				Agent:  "reviewer",
				Prompt: "",
			},
			Expected: base.ToolTestExpectation[*SpawnTaskResult]{
				Error: base.NewCustomError("prompt is required", []string{
					"Provide the initial instructions for the subtask",
				}),
			},
		},
		{
			Name: "parent task does not exist",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				test.NewAgentBuilder(t, targetAgentID, db, model).
					WithName("reviewer").
					Build(ctx)
			},
			TestInput: &SpawnTaskInput{
				Agent:  "reviewer",
				Prompt: "Do something",
			},
			Expected: base.ToolTestExpectation[*SpawnTaskResult]{
				Error: base.NewCustomError("failed to get current task: task not found", []string{}),
			},
		},
	})
}
