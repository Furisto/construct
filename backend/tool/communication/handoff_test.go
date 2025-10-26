package communication

import (
	"context"
	"testing"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/message"
	"github.com/furisto/construct/backend/memory/test"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	_ "modernc.org/sqlite"
)

func TestHandoff(t *testing.T) {
	t.Parallel()

	type DatabaseResult struct {
		AssignedAgent   uuid.UUID
		HandoverMessage string
	}

	sourceAgentID := uuid.New()
	targetAgentID := uuid.New()
	taskID := uuid.New()

	setup := &base.ToolTestSetup[*HandoffInput, struct{}]{
		Call: func(ctx context.Context, services *base.ToolTestServices, input *HandoffInput) (struct{}, error) {
			return struct{}{}, Handoff(ctx, services.DB, input)
		},
		QueryDatabase: func(ctx context.Context, db *memory.Client) (any, error) {
			var result DatabaseResult
			task, err := db.Task.Get(ctx, taskID)
			if err == nil {
				result.AssignedAgent = task.AgentID
			}

			handoverMessage, err := db.Message.Query().Where(message.TaskIDEQ(taskID)).Order(message.ByCreateTime()).First(ctx)
			if err == nil && handoverMessage != nil && handoverMessage.Content != nil && len(handoverMessage.Content.Blocks) > 0 {
				result.HandoverMessage = handoverMessage.Content.Blocks[0].Payload
			}

			return result, nil
		},
		CmpOptions: []cmp.Option{
			cmpopts.IgnoreFields(base.ToolError{}, "Suggestions"),
		},
	}

	setup.RunToolTests(t, []base.ToolTestScenario[*HandoffInput, struct{}]{
		{
			Name: "source agent does not exist",
			TestInput: &HandoffInput{
				TaskID:         taskID,
				CurrentAgentID: sourceAgentID,
				RequestedAgent: "source",
			},
			Expected: base.ToolTestExpectation[struct{}]{
				Error: base.NewCustomError("failed to get current agent: agent not found", []string{}),
			},
		},
		{
			Name: "target agent does not exist",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)
				sourceAgent := test.NewAgentBuilder(t, sourceAgentID, db, model).
					WithName("source").
					Build(ctx)

				test.NewTaskBuilder(t, taskID, db, sourceAgent).Build(ctx)
			},
			TestInput: &HandoffInput{
				TaskID:         taskID,
				CurrentAgentID: sourceAgentID,
				RequestedAgent: "target",
			},
			Expected: base.ToolTestExpectation[struct{}]{
				Error: base.NewCustomError("agent target does not exist", []string{
					"Check the agent name and try again",
				}),
			},
		},
		{
			Name: "task does not exist",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				test.NewAgentBuilder(t, sourceAgentID, db, model).
					WithName("source").
					Build(ctx)

				test.NewAgentBuilder(t, targetAgentID, db, model).
					WithName("target").
					Build(ctx)
			},
			TestInput: &HandoffInput{
				TaskID:         taskID,
				CurrentAgentID: sourceAgentID,
				RequestedAgent: "target",
			},
			Expected: base.ToolTestExpectation[struct{}]{
				Error: base.NewCustomError("failed to get task: task not found", []string{}),
			},
		},
		{
			Name: "successful handoff without handover message",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				sourceAgent := test.NewAgentBuilder(t, sourceAgentID, db, model).
					WithName("source").
					Build(ctx)

				test.NewAgentBuilder(t, targetAgentID, db, model).
					WithName("target").
					Build(ctx)

				test.NewTaskBuilder(t, taskID, db, sourceAgent).Build(ctx)
			},
			TestInput: &HandoffInput{
				TaskID:         taskID,
				CurrentAgentID: sourceAgentID,
				RequestedAgent: "target",
			},
			Expected: base.ToolTestExpectation[struct{}]{
				Database: DatabaseResult{
					AssignedAgent:   targetAgentID,
					HandoverMessage: "The source agent has performed a handoff to you, the target agent.\n\n",
				},
			},
		},
		{
			Name: "successful handoff with handover message",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				sourceAgent := test.NewAgentBuilder(t, sourceAgentID, db, model).
					WithName("source").
					Build(ctx)

				test.NewAgentBuilder(t, targetAgentID, db, model).
					WithName("target").
					Build(ctx)

				test.NewTaskBuilder(t, taskID, db, sourceAgent).Build(ctx)
			},
			TestInput: &HandoffInput{
				TaskID:          taskID,
				CurrentAgentID:  sourceAgentID,
				RequestedAgent:  "target",
				HandoverMessage: "handover message",
			},
			Expected: base.ToolTestExpectation[struct{}]{
				Database: DatabaseResult{
					AssignedAgent:   targetAgentID,
					HandoverMessage: "The source agent has performed a handoff to you, the target agent.\n\nIt has left the following instructions for you:\nhandover message",
				},
			},
		},
		{
			Name: "handoff to same agent",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, uuid.New(), db).Build(ctx)
				model := test.NewModelBuilder(t, uuid.New(), db, modelProvider).Build(ctx)

				sourceAgent := test.NewAgentBuilder(t, sourceAgentID, db, model).
					WithName("source").
					Build(ctx)

				test.NewTaskBuilder(t, taskID, db, sourceAgent).Build(ctx)
			},
			TestInput: &HandoffInput{
				TaskID:         taskID,
				CurrentAgentID: sourceAgentID,
				RequestedAgent: "source",
			},
			Expected: base.ToolTestExpectation[struct{}]{
				Error: base.NewCustomError("agent cannot handoff to itself", []string{}),
			},
		},
	})
}
