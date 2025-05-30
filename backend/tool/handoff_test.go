package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/message"
	"github.com/furisto/construct/backend/memory/test"
	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	_ "github.com/mattn/go-sqlite3"
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

	setup := &ToolTestSetup[HandoffInput, struct{}]{
		Call: func(ctx context.Context, db *memory.Client, input *HandoffInput) (struct{}, error) {
			return struct{}{}, handoff(ctx, db, input)
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
			cmp.Transformer("ErrorMessageOnly", func(err string) string {
				var errorObj map[string]interface{}
				if json.Unmarshal([]byte(err), &errorObj) != nil {
					return err
				}
				if msg, ok := errorObj["Message"]; ok {
					if msgStr, ok := msg.(string); ok {
						return msgStr
					}
				}
				return err
			}),
		},
	}

	setup.RunToolTests(t, []ToolTestScenario[HandoffInput, struct{}]{
		{
			Name: "source agent does not exist",
			TestInput: &HandoffInput{
				TaskID:         taskID,
				CurrentAgentID: sourceAgentID,
				RequestedAgent: "source",
			},
			Expected: ToolTestExpectation[struct{}]{
				Error: codeact.NewCustomError("failed to get current agent: agent not found", []string{}).Error(),
			},
		},
		{
			Name: "target agent does not exist",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)
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
			Expected: ToolTestExpectation[struct{}]{
				Error: codeact.NewCustomError("agent target does not exist", []string{
					"Check the agent name and try again",
				}).Error(),
			},
		},
		{
			Name: "task does not exist",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

				test.NewAgentBuilder(t, sourceAgentID, db, model).
					WithName("source").
					Build(ctx)

				test.NewAgentBuilder(t, targetAgentID, db, model).
					WithID(test.AgentID2()).
					WithName("target").
					Build(ctx)
			},
			TestInput: &HandoffInput{
				TaskID:         taskID,
				CurrentAgentID: sourceAgentID,
				RequestedAgent: "target",
			},
			Expected: ToolTestExpectation[struct{}]{
				Error: "memory: task not found",
			},
		},
		{
			Name: "successful handoff without handover message",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

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
			Expected: ToolTestExpectation[struct{}]{
				Database: DatabaseResult{
					AssignedAgent:   targetAgentID,
					HandoverMessage: "The source agent has performed a handoff to you, the target agent.\n\n",
				},
			},
		},
		{
			Name: "successful handoff with handover message",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

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
			Expected: ToolTestExpectation[struct{}]{
				Database: DatabaseResult{
					AssignedAgent:   targetAgentID,
					HandoverMessage: "The source agent has performed a handoff to you, the target agent.\n\nIt has left the following instructions for you:\nhandover message",
				},
			},
		},
		{
			Name: "handoff to same agent",
			SeedDatabase: func(ctx context.Context, db *memory.Client) {
				modelProvider := test.NewModelProviderBuilder(t, db).Build(ctx)
				model := test.NewModelBuilder(t, db, modelProvider).Build(ctx)

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
			Expected: ToolTestExpectation[struct{}]{
				Error: "agent cannot handoff to itself",
			},
		},
	})
}
