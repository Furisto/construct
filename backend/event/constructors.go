package event

import (
	"time"

	"github.com/furisto/construct/backend/memory"
	tooltypes "github.com/furisto/construct/backend/tool/types"
	"github.com/google/uuid"
)

// Event type constants
const (
	// Task events
	EventTypeTaskCreated = "task.created"
	EventTypeTaskUpdated = "task.updated"
	EventTypeTaskDeleted = "task.deleted"

	// Message events
	EventTypeMessageCreated = "message.created"
	EventTypeMessageUpdated = "message.updated"
	EventTypeMessageDeleted = "message.deleted"
	EventTypeMessageChunk   = "message.chunk"

	// Agent events
	EventTypeAgentCreated = "agent.created"
	EventTypeAgentUpdated = "agent.updated"
	EventTypeAgentDeleted = "agent.deleted"

	// Model events
	EventTypeModelCreated = "model.created"
	EventTypeModelUpdated = "model.updated"
	EventTypeModelDeleted = "model.deleted"

	// ModelProvider events
	EventTypeModelProviderCreated = "modelprovider.created"
	EventTypeModelProviderUpdated = "modelprovider.updated"
	EventTypeModelProviderDeleted = "modelprovider.deleted"

	// Tool events
	EventTypeToolCalled = "tool.called"
	EventTypeToolResult = "tool.result"
)

// Event action constants
const (
	ActionCreated = "created"
	ActionUpdated = "updated"
	ActionDeleted = "deleted"
)

// TaskEventPayload contains the payload for task events.
type TaskEventPayload struct {
	Task          *memory.Task
	PreviousPhase string // Only set for task.updated events
}

// MessageEventPayload contains the payload for message events.
type MessageEventPayload struct {
	Message *memory.Message
}

// MessageChunkPayload contains the payload for message.chunk events.
type MessageChunkPayload struct {
	TaskID     uuid.UUID
	MessageID  uuid.UUID
	Chunk      string
	ChunkIndex int
}

// AgentEventPayload contains the payload for agent events.
type AgentEventPayload struct {
	Agent *memory.Agent
}

// ModelEventPayload contains the payload for model events.
type ModelEventPayload struct {
	Model *memory.Model
}

// ModelProviderEventPayload contains the payload for model provider events.
type ModelProviderEventPayload struct {
	ModelProvider *memory.ModelProvider
}

// DeletedEntityPayload contains the payload for deleted entity events.
type DeletedEntityPayload struct {
	ID     uuid.UUID
	TaskID *uuid.UUID // Only set for message.deleted events
}

// --- Task Event Constructors ---

// NewTaskCreatedEvent creates a new task.created event.
func NewTaskCreatedEvent(task *memory.Task) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeTaskCreated,
		Action:    ActionCreated,
		Timestamp: time.Now(),
		TaskID:    &task.ID,
		Payload: &TaskEventPayload{
			Task: task,
		},
	}
}

// NewTaskUpdatedEvent creates a new task.updated event.
func NewTaskUpdatedEvent(task *memory.Task, previousPhase string) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeTaskUpdated,
		Action:    ActionUpdated,
		Timestamp: time.Now(),
		TaskID:    &task.ID,
		Payload: &TaskEventPayload{
			Task:          task,
			PreviousPhase: previousPhase,
		},
	}
}

// NewTaskDeletedEvent creates a new task.deleted event.
func NewTaskDeletedEvent(taskID uuid.UUID) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeTaskDeleted,
		Action:    ActionDeleted,
		Timestamp: time.Now(),
		TaskID:    &taskID,
		Payload: &DeletedEntityPayload{
			ID: taskID,
		},
	}
}

// --- Message Event Constructors ---

// NewMessageCreatedEvent creates a new message.created event.
func NewMessageCreatedEvent(message *memory.Message) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeMessageCreated,
		Action:    ActionCreated,
		Timestamp: time.Now(),
		TaskID:    &message.TaskID,
		Payload: &MessageEventPayload{
			Message: message,
		},
	}
}

// NewMessageUpdatedEvent creates a new message.updated event.
func NewMessageUpdatedEvent(message *memory.Message) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeMessageUpdated,
		Action:    ActionUpdated,
		Timestamp: time.Now(),
		TaskID:    &message.TaskID,
		Payload: &MessageEventPayload{
			Message: message,
		},
	}
}

// NewMessageDeletedEvent creates a new message.deleted event.
func NewMessageDeletedEvent(messageID, taskID uuid.UUID) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeMessageDeleted,
		Action:    ActionDeleted,
		Timestamp: time.Now(),
		TaskID:    &taskID,
		Payload: &DeletedEntityPayload{
			ID:     messageID,
			TaskID: &taskID,
		},
	}
}

// NewMessageChunkEvent creates a new message.chunk event for streaming partial content.
func NewMessageChunkEvent(taskID, messageID uuid.UUID, chunk string, chunkIndex int) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeMessageChunk,
		Action:    ActionCreated,
		Timestamp: time.Now(),
		TaskID:    &taskID,
		Payload: &MessageChunkPayload{
			TaskID:     taskID,
			MessageID:  messageID,
			Chunk:      chunk,
			ChunkIndex: chunkIndex,
		},
	}
}

// --- Agent Event Constructors ---

// NewAgentCreatedEvent creates a new agent.created event.
func NewAgentCreatedEvent(agent *memory.Agent) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeAgentCreated,
		Action:    ActionCreated,
		Timestamp: time.Now(),
		Payload: &AgentEventPayload{
			Agent: agent,
		},
	}
}

// NewAgentUpdatedEvent creates a new agent.updated event.
func NewAgentUpdatedEvent(agent *memory.Agent) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeAgentUpdated,
		Action:    ActionUpdated,
		Timestamp: time.Now(),
		Payload: &AgentEventPayload{
			Agent: agent,
		},
	}
}

// NewAgentDeletedEvent creates a new agent.deleted event.
func NewAgentDeletedEvent(agentID uuid.UUID) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeAgentDeleted,
		Action:    ActionDeleted,
		Timestamp: time.Now(),
		Payload: &DeletedEntityPayload{
			ID: agentID,
		},
	}
}

// --- Model Event Constructors ---

// NewModelCreatedEvent creates a new model.created event.
func NewModelCreatedEvent(model *memory.Model) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeModelCreated,
		Action:    ActionCreated,
		Timestamp: time.Now(),
		Payload: &ModelEventPayload{
			Model: model,
		},
	}
}

// NewModelUpdatedEvent creates a new model.updated event.
func NewModelUpdatedEvent(model *memory.Model) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeModelUpdated,
		Action:    ActionUpdated,
		Timestamp: time.Now(),
		Payload: &ModelEventPayload{
			Model: model,
		},
	}
}

// NewModelDeletedEvent creates a new model.deleted event.
func NewModelDeletedEvent(modelID uuid.UUID) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeModelDeleted,
		Action:    ActionDeleted,
		Timestamp: time.Now(),
		Payload: &DeletedEntityPayload{
			ID: modelID,
		},
	}
}

// --- ModelProvider Event Constructors ---

// NewModelProviderCreatedEvent creates a new modelprovider.created event.
func NewModelProviderCreatedEvent(provider *memory.ModelProvider) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeModelProviderCreated,
		Action:    ActionCreated,
		Timestamp: time.Now(),
		Payload: &ModelProviderEventPayload{
			ModelProvider: provider,
		},
	}
}

// NewModelProviderUpdatedEvent creates a new modelprovider.updated event.
func NewModelProviderUpdatedEvent(provider *memory.ModelProvider) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeModelProviderUpdated,
		Action:    ActionUpdated,
		Timestamp: time.Now(),
		Payload: &ModelProviderEventPayload{
			ModelProvider: provider,
		},
	}
}

// NewModelProviderDeletedEvent creates a new modelprovider.deleted event.
func NewModelProviderDeletedEvent(providerID uuid.UUID) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeModelProviderDeleted,
		Action:    ActionDeleted,
		Timestamp: time.Now(),
		Payload: &DeletedEntityPayload{
			ID: providerID,
		},
	}
}

// --- Tool Event Constructors ---

// NewToolCalledEvent creates a new tool.called event.
// This is a transient streaming event and is NOT replayed.
func NewToolCalledEvent(taskID uuid.UUID, evt tooltypes.ToolCallEvent) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeToolCalled,
		Action:    ActionCreated,
		Timestamp: time.Now(),
		TaskID:    &taskID,
		Payload:   &evt,
	}
}

// NewToolResultEvent creates a new tool.result event.
// This is a transient streaming event and is NOT replayed.
func NewToolResultEvent(taskID uuid.UUID, evt tooltypes.ToolResultEvent) *StreamEvent {
	return &StreamEvent{
		Type:      EventTypeToolResult,
		Action:    ActionCreated,
		Timestamp: time.Now(),
		TaskID:    &taskID,
		Payload:   &evt,
	}
}
