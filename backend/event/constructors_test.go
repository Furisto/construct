package event

import (
	"testing"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/tool/filesystem"
	tooltypes "github.com/furisto/construct/backend/tool/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
)

var cmpOpts = []cmp.Option{
	cmpopts.IgnoreFields(StreamEvent{}, "Timestamp"),
	cmpopts.IgnoreUnexported(
		memory.Task{},
		memory.TaskEdges{},
		memory.Message{},
		memory.MessageEdges{},
		memory.Agent{},
		memory.AgentEdges{},
		memory.Model{},
		memory.ModelEdges{},
		memory.ModelProvider{},
		memory.ModelProviderEdges{},
	),
}

func TestNewTaskCreatedEvent(t *testing.T) {
	taskID := uuid.New()
	task := &memory.Task{ID: taskID}

	got := NewTaskCreatedEvent(task)

	want := &StreamEvent{
		Type:    EventTypeTaskCreated,
		Action:  ActionCreated,
		TaskID:  &taskID,
		Payload: &TaskEventPayload{Task: task},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewTaskCreatedEvent() mismatch (-want +got):\n%s", diff)
	}

	if got.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestNewTaskUpdatedEvent(t *testing.T) {
	taskID := uuid.New()
	task := &memory.Task{ID: taskID}
	previousPhase := "running"

	got := NewTaskUpdatedEvent(task, previousPhase)

	want := &StreamEvent{
		Type:   EventTypeTaskUpdated,
		Action: ActionUpdated,
		TaskID: &taskID,
		Payload: &TaskEventPayload{
			Task:          task,
			PreviousPhase: previousPhase,
		},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewTaskUpdatedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewTaskDeletedEvent(t *testing.T) {
	taskID := uuid.New()

	got := NewTaskDeletedEvent(taskID)

	want := &StreamEvent{
		Type:    EventTypeTaskDeleted,
		Action:  ActionDeleted,
		TaskID:  &taskID,
		Payload: &DeletedEntityPayload{ID: taskID},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewTaskDeletedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewMessageCreatedEvent(t *testing.T) {
	taskID := uuid.New()
	messageID := uuid.New()
	message := &memory.Message{ID: messageID, TaskID: taskID}

	got := NewMessageCreatedEvent(message)

	want := &StreamEvent{
		Type:    EventTypeMessageCreated,
		Action:  ActionCreated,
		TaskID:  &taskID,
		Payload: &MessageEventPayload{Message: message},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewMessageCreatedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewMessageUpdatedEvent(t *testing.T) {
	taskID := uuid.New()
	messageID := uuid.New()
	message := &memory.Message{ID: messageID, TaskID: taskID}

	got := NewMessageUpdatedEvent(message)

	want := &StreamEvent{
		Type:    EventTypeMessageUpdated,
		Action:  ActionUpdated,
		TaskID:  &taskID,
		Payload: &MessageEventPayload{Message: message},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewMessageUpdatedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewMessageDeletedEvent(t *testing.T) {
	taskID := uuid.New()
	messageID := uuid.New()

	got := NewMessageDeletedEvent(messageID, taskID)

	want := &StreamEvent{
		Type:   EventTypeMessageDeleted,
		Action: ActionDeleted,
		TaskID: &taskID,
		Payload: &DeletedEntityPayload{
			ID:     messageID,
			TaskID: &taskID,
		},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewMessageDeletedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewMessageChunkEvent(t *testing.T) {
	taskID := uuid.New()
	messageID := uuid.New()
	chunk := "partial content"
	chunkIndex := 5

	got := NewMessageChunkEvent(taskID, messageID, chunk, chunkIndex)

	want := &StreamEvent{
		Type:   EventTypeMessageChunk,
		Action: ActionCreated,
		TaskID: &taskID,
		Payload: &MessageChunkPayload{
			TaskID:     taskID,
			MessageID:  messageID,
			Chunk:      chunk,
			ChunkIndex: chunkIndex,
		},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewMessageChunkEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewAgentCreatedEvent(t *testing.T) {
	agentID := uuid.New()
	agent := &memory.Agent{ID: agentID}

	got := NewAgentCreatedEvent(agent)

	want := &StreamEvent{
		Type:    EventTypeAgentCreated,
		Action:  ActionCreated,
		TaskID:  nil,
		Payload: &AgentEventPayload{Agent: agent},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewAgentCreatedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewAgentUpdatedEvent(t *testing.T) {
	agentID := uuid.New()
	agent := &memory.Agent{ID: agentID}

	got := NewAgentUpdatedEvent(agent)

	want := &StreamEvent{
		Type:    EventTypeAgentUpdated,
		Action:  ActionUpdated,
		TaskID:  nil,
		Payload: &AgentEventPayload{Agent: agent},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewAgentUpdatedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewAgentDeletedEvent(t *testing.T) {
	agentID := uuid.New()

	got := NewAgentDeletedEvent(agentID)

	want := &StreamEvent{
		Type:    EventTypeAgentDeleted,
		Action:  ActionDeleted,
		TaskID:  nil,
		Payload: &DeletedEntityPayload{ID: agentID},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewAgentDeletedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewModelCreatedEvent(t *testing.T) {
	modelID := uuid.New()
	model := &memory.Model{ID: modelID}

	got := NewModelCreatedEvent(model)

	want := &StreamEvent{
		Type:    EventTypeModelCreated,
		Action:  ActionCreated,
		TaskID:  nil,
		Payload: &ModelEventPayload{Model: model},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewModelCreatedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewModelUpdatedEvent(t *testing.T) {
	modelID := uuid.New()
	model := &memory.Model{ID: modelID}

	got := NewModelUpdatedEvent(model)

	want := &StreamEvent{
		Type:    EventTypeModelUpdated,
		Action:  ActionUpdated,
		TaskID:  nil,
		Payload: &ModelEventPayload{Model: model},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewModelUpdatedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewModelDeletedEvent(t *testing.T) {
	modelID := uuid.New()

	got := NewModelDeletedEvent(modelID)

	want := &StreamEvent{
		Type:    EventTypeModelDeleted,
		Action:  ActionDeleted,
		TaskID:  nil,
		Payload: &DeletedEntityPayload{ID: modelID},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewModelDeletedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewModelProviderCreatedEvent(t *testing.T) {
	providerID := uuid.New()
	provider := &memory.ModelProvider{ID: providerID}

	got := NewModelProviderCreatedEvent(provider)

	want := &StreamEvent{
		Type:    EventTypeModelProviderCreated,
		Action:  ActionCreated,
		TaskID:  nil,
		Payload: &ModelProviderEventPayload{ModelProvider: provider},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewModelProviderCreatedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewModelProviderUpdatedEvent(t *testing.T) {
	providerID := uuid.New()
	provider := &memory.ModelProvider{ID: providerID}

	got := NewModelProviderUpdatedEvent(provider)

	want := &StreamEvent{
		Type:    EventTypeModelProviderUpdated,
		Action:  ActionUpdated,
		TaskID:  nil,
		Payload: &ModelProviderEventPayload{ModelProvider: provider},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewModelProviderUpdatedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewModelProviderDeletedEvent(t *testing.T) {
	providerID := uuid.New()

	got := NewModelProviderDeletedEvent(providerID)

	want := &StreamEvent{
		Type:    EventTypeModelProviderDeleted,
		Action:  ActionDeleted,
		TaskID:  nil,
		Payload: &DeletedEntityPayload{ID: providerID},
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewModelProviderDeletedEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewToolCalledEvent(t *testing.T) {
	taskID := uuid.New()
	toolCallEvent := tooltypes.ToolCallEvent{
		ID:   "call_123",
		Tool: "read_file",
		Input: tooltypes.ToolInput{
			ReadFile: &filesystem.ReadFileInput{
				Path: "/path/to/file",
			},
		},
	}

	got := NewToolCalledEvent(taskID, toolCallEvent)

	want := &StreamEvent{
		Type:    EventTypeToolCalled,
		Action:  ActionCreated,
		TaskID:  &taskID,
		Payload: &toolCallEvent,
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewToolCalledEvent() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewToolResultEvent(t *testing.T) {
	taskID := uuid.New()
	toolResultEvent := tooltypes.ToolResultEvent{
		ID:   "call_123",
		Tool: "read_file",
		Output: tooltypes.ToolOutput{
			ReadFile: &filesystem.ReadFileResult{
				Content: "file content",
			},
		},
	}

	got := NewToolResultEvent(taskID, toolResultEvent)

	want := &StreamEvent{
		Type:    EventTypeToolResult,
		Action:  ActionCreated,
		TaskID:  &taskID,
		Payload: &toolResultEvent,
	}

	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("NewToolResultEvent() mismatch (-want +got):\n%s", diff)
	}
}
