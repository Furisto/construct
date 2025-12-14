package subtask

import (
	"context"
	"fmt"
	"strings"

	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/agent"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/google/uuid"
)

func SpawnTask(ctx context.Context, db *memory.Client, bus *event.Bus, currentTaskID uuid.UUID, input *SpawnTaskInput) (*SpawnTaskResult, error) {
	if input.Agent == "" {
		return nil, base.NewCustomError("agent is required", []string{
			"Provide the name or ID of the agent to assign to the subtask",
		})
	}

	if input.Prompt == "" {
		return nil, base.NewCustomError("prompt is required", []string{
			"Provide the initial instructions for the subtask",
		})
	}

	result, err := memory.Transaction(ctx, db, func(tx *memory.Client) (*SpawnTaskResult, error) {
		currentTask, err := tx.Task.Get(ctx, currentTaskID)
		if err != nil {
			return nil, base.NewCustomError(fmt.Sprintf("failed to get current task: %v", memory.SanitizeError(err)), []string{
				"This is likely a system bug. Ask the user how to proceed",
			})
		}

		resolvedAgent, err := resolveAgent(ctx, db, input.Agent)
		if err != nil {
			return nil, err
		}

		subtask, err := tx.Task.Create().
			SetAgentID(resolvedAgent.ID).
			SetProjectDirectory(currentTask.ProjectDirectory).
			SetParentTaskID(currentTaskID).
			SetDesiredPhase(types.TaskPhaseSuspended).
			Save(ctx)
		if err != nil {
			return nil, base.NewCustomError(fmt.Sprintf("failed to create subtask: %v", memory.SanitizeError(err)), []string{
				"This is likely a system bug. Ask the user how to proceed",
			})
		}

		_, err = tx.Message.Create().
			SetTaskID(subtask.ID).
			SetSource(types.MessageSourceUser).
			SetContent(&types.MessageContent{
				Blocks: []types.MessageBlock{
					{
						Kind:    types.MessageBlockKindText,
						Payload: input.Prompt,
					},
				},
			}).
			Save(ctx)
		if err != nil {
			return nil, base.NewCustomError(fmt.Sprintf("failed to create initial message: %v", memory.SanitizeError(err)), []string{
				"This is likely a system bug. Ask the user how to proceed",
			})
		}

		event.Publish(bus, event.TaskReconciliationEvent{TaskID: subtask.ID})

		return &SpawnTaskResult{
			TaskID: subtask.ID.String(),
		}, nil
	})

	return result, err
}

func resolveAgent(ctx context.Context, db *memory.Client, agentName string) (*memory.Agent, error) {
	resolvedAgent, err := db.Agent.Query().Where(agent.NameEQ(agentName)).First(ctx)
	if err != nil {
		if memory.IsNotFound(err) {

			var suggestions []string
			agents, err := db.Agent.Query().Select(agent.FieldName).All(ctx)
			if err == nil {
				agentNames := make([]string, len(agents))
				for i, agent := range agents {
					agentNames[i] = agent.Name
				}

				suggestions = append(suggestions, fmt.Sprintf("Agent %s does not exist. Available agents: %s", agentName, strings.Join(agentNames, ", ")))
			}

			return nil, base.NewCustomError(fmt.Sprintf("agent with name %s does not exist", agentName), suggestions)
		}
		return nil, err
	}

	return resolvedAgent, nil
}
