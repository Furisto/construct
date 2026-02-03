package event

import (
	"context"
	"log/slog"

	"github.com/furisto/construct/backend/memory"
	"github.com/google/uuid"
)

// RegisterHooks registers ent hooks on the memory client to publish events
// when entities are created, updated, or deleted.
//
// Events published:
//   - agent.created, agent.updated, agent.deleted
//   - model.created, model.updated, model.deleted
//   - modelprovider.created, modelprovider.updated, modelprovider.deleted
//   - task.created, task.deleted (NOT task.updated - that's from reconciler)
func RegisterHooks(client *memory.Client, router *EventRouter) {
	client.Agent.Use(agentHook(router))
	client.Model.Use(modelHook(router))
	client.ModelProvider.Use(modelProviderHook(router))
	client.Task.Use(taskHook(router))
}

// agentHook creates a hook for Agent mutations.
func agentHook(router *EventRouter) memory.Hook {
	return func(next memory.Mutator) memory.Mutator {
		return memory.MutateFunc(func(ctx context.Context, m memory.Mutation) (memory.Value, error) {
			mutation, ok := m.(*memory.AgentMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}

			// For delete operations, capture ID before mutation
			var deleteID uuid.UUID
			if mutation.Op().Is(memory.OpDelete | memory.OpDeleteOne) {
				if id, exists := mutation.ID(); exists {
					deleteID = id
				}
			}

			// Execute mutation
			value, err := next.Mutate(ctx, m)
			if err != nil {
				return value, err
			}

			// Publish event based on operation
			switch {
			case mutation.Op().Is(memory.OpCreate):
				if agent, ok := value.(*memory.Agent); ok {
					router.Publish(NewAgentCreatedEvent(agent))
				}
			case mutation.Op().Is(memory.OpUpdate | memory.OpUpdateOne):
				if agent, ok := value.(*memory.Agent); ok {
					router.Publish(NewAgentUpdatedEvent(agent))
				}
			case mutation.Op().Is(memory.OpDelete | memory.OpDeleteOne):
				if deleteID != uuid.Nil {
					router.Publish(NewAgentDeletedEvent(deleteID))
				} else {
					slog.Warn("agent deleted but ID was not captured")
				}
			}

			return value, nil
		})
	}
}

// modelHook creates a hook for Model mutations.
func modelHook(router *EventRouter) memory.Hook {
	return func(next memory.Mutator) memory.Mutator {
		return memory.MutateFunc(func(ctx context.Context, m memory.Mutation) (memory.Value, error) {
			mutation, ok := m.(*memory.ModelMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}

			// For delete operations, capture ID before mutation
			var deleteID uuid.UUID
			if mutation.Op().Is(memory.OpDelete | memory.OpDeleteOne) {
				if id, exists := mutation.ID(); exists {
					deleteID = id
				}
			}

			// Execute mutation
			value, err := next.Mutate(ctx, m)
			if err != nil {
				return value, err
			}

			// Publish event based on operation
			switch {
			case mutation.Op().Is(memory.OpCreate):
				if model, ok := value.(*memory.Model); ok {
					router.Publish(NewModelCreatedEvent(model))
				}
			case mutation.Op().Is(memory.OpUpdate | memory.OpUpdateOne):
				if model, ok := value.(*memory.Model); ok {
					router.Publish(NewModelUpdatedEvent(model))
				}
			case mutation.Op().Is(memory.OpDelete | memory.OpDeleteOne):
				if deleteID != uuid.Nil {
					router.Publish(NewModelDeletedEvent(deleteID))
				} else {
					slog.Warn("model deleted but ID was not captured")
				}
			}

			return value, nil
		})
	}
}

// modelProviderHook creates a hook for ModelProvider mutations.
func modelProviderHook(router *EventRouter) memory.Hook {
	return func(next memory.Mutator) memory.Mutator {
		return memory.MutateFunc(func(ctx context.Context, m memory.Mutation) (memory.Value, error) {
			mutation, ok := m.(*memory.ModelProviderMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}

			// For delete operations, capture ID before mutation
			var deleteID uuid.UUID
			if mutation.Op().Is(memory.OpDelete | memory.OpDeleteOne) {
				if id, exists := mutation.ID(); exists {
					deleteID = id
				}
			}

			// Execute mutation
			value, err := next.Mutate(ctx, m)
			if err != nil {
				return value, err
			}

			// Publish event based on operation
			switch {
			case mutation.Op().Is(memory.OpCreate):
				if provider, ok := value.(*memory.ModelProvider); ok {
					router.Publish(NewModelProviderCreatedEvent(provider))
				}
			case mutation.Op().Is(memory.OpUpdate | memory.OpUpdateOne):
				if provider, ok := value.(*memory.ModelProvider); ok {
					router.Publish(NewModelProviderUpdatedEvent(provider))
				}
			case mutation.Op().Is(memory.OpDelete | memory.OpDeleteOne):
				if deleteID != uuid.Nil {
					router.Publish(NewModelProviderDeletedEvent(deleteID))
				} else {
					slog.Warn("model provider deleted but ID was not captured")
				}
			}

			return value, nil
		})
	}
}

// taskHook creates a hook for Task mutations.
// Note: task.updated events are NOT emitted from hooks - they are emitted from
// the TaskReconciler when phase/stats changes occur.
func taskHook(router *EventRouter) memory.Hook {
	return func(next memory.Mutator) memory.Mutator {
		return memory.MutateFunc(func(ctx context.Context, m memory.Mutation) (memory.Value, error) {
			mutation, ok := m.(*memory.TaskMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}

			// For delete operations, capture ID before mutation
			var deleteID uuid.UUID
			if mutation.Op().Is(memory.OpDelete | memory.OpDeleteOne) {
				if id, exists := mutation.ID(); exists {
					deleteID = id
				}
			}

			// Execute mutation
			value, err := next.Mutate(ctx, m)
			if err != nil {
				return value, err
			}

			// Publish event based on operation
			// Note: We do NOT emit task.updated from hooks - that's from reconciler
			switch {
			case mutation.Op().Is(memory.OpCreate):
				if task, ok := value.(*memory.Task); ok {
					router.Publish(NewTaskCreatedEvent(task))
				}
			case mutation.Op().Is(memory.OpDelete | memory.OpDeleteOne):
				if deleteID != uuid.Nil {
					router.Publish(NewTaskDeletedEvent(deleteID))
				} else {
					slog.Warn("task deleted but ID was not captured")
				}
			}

			return value, nil
		})
	}
}
