package event

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/furisto/construct/backend/memory"
	_ "github.com/furisto/construct/backend/memory/runtime"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func setupTestDB(t *testing.T) *memory.Client {
	t.Helper()
	client, err := memory.Open(dialect.SQLite, "file:construct_test?mode=memory&cache=private&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
	})
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	return client
}

func TestRegisterHooks_AgentCreated(t *testing.T) {
	client := setupTestDB(t)
	router := NewEventRouter(10)
	defer router.Close()

	RegisterHooks(client, router)

	ctx := context.Background()
	ch, cancel := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"agent.created"}})
	defer cancel()

	// Create an agent
	agent, err := client.Agent.Create().
		SetName("test-agent").
		SetDescription("Test agent description").
		SetInstructions("Test instructions").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Wait for event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeAgentCreated, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(ActionCreated, event.Action); diff != "" {
			t.Errorf("action mismatch (-want +got):\n%s", diff)
		}
		payload, ok := event.Payload.(*AgentEventPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", event.Payload)
		}
		if diff := cmp.Diff(agent.ID, payload.Agent.ID); diff != "" {
			t.Errorf("agent ID mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestRegisterHooks_AgentUpdated(t *testing.T) {
	client := setupTestDB(t)
	router := NewEventRouter(10)
	defer router.Close()

	RegisterHooks(client, router)

	ctx := context.Background()

	// Create agent first (before subscribing)
	agent, err := client.Agent.Create().
		SetName("test-agent").
		SetDescription("Test agent description").
		SetInstructions("Test instructions").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	ch, cancel := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"agent.updated"}})
	defer cancel()

	// Update the agent
	agent, err = client.Agent.UpdateOne(agent).
		SetDescription("Updated description").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to update agent: %v", err)
	}

	// Wait for event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeAgentUpdated, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(ActionUpdated, event.Action); diff != "" {
			t.Errorf("action mismatch (-want +got):\n%s", diff)
		}
		payload, ok := event.Payload.(*AgentEventPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", event.Payload)
		}
		if diff := cmp.Diff(agent.ID, payload.Agent.ID); diff != "" {
			t.Errorf("agent ID mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestRegisterHooks_AgentDeleted(t *testing.T) {
	client := setupTestDB(t)
	router := NewEventRouter(10)
	defer router.Close()

	RegisterHooks(client, router)

	ctx := context.Background()

	// Create agent first
	agent, err := client.Agent.Create().
		SetName("test-agent").
		SetDescription("Test agent description").
		SetInstructions("Test instructions").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	agentID := agent.ID

	ch, cancel := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"agent.deleted"}})
	defer cancel()

	// Delete the agent
	err = client.Agent.DeleteOne(agent).Exec(ctx)
	if err != nil {
		t.Fatalf("failed to delete agent: %v", err)
	}

	// Wait for event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeAgentDeleted, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(ActionDeleted, event.Action); diff != "" {
			t.Errorf("action mismatch (-want +got):\n%s", diff)
		}
		payload, ok := event.Payload.(*DeletedEntityPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", event.Payload)
		}
		if diff := cmp.Diff(agentID, payload.ID); diff != "" {
			t.Errorf("deleted agent ID mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestRegisterHooks_ModelCreatedUpdatedDeleted(t *testing.T) {
	client := setupTestDB(t)
	router := NewEventRouter(10)
	defer router.Close()

	RegisterHooks(client, router)

	ctx := context.Background()

	// Subscribe to all model events
	ch, cancel := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"model.*"}})
	defer cancel()

	// Create provider first (required for model)
	provider, err := client.ModelProvider.Create().
		SetName("test-provider").
		SetProviderType(types.ModelProviderTypeOpenAI).
		SetSecret([]byte("test-secret")).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Create model
	model, err := client.Model.Create().
		SetName("gpt-4").
		SetContextWindow(128000).
		SetModelProvider(provider).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	// Verify model.created event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeModelCreated, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for model.created event")
	}

	// Update model
	model, err = client.Model.UpdateOne(model).
		SetName("gpt-4-turbo").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to update model: %v", err)
	}

	// Verify model.updated event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeModelUpdated, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for model.updated event")
	}

	// Delete model
	err = client.Model.DeleteOne(model).Exec(ctx)
	if err != nil {
		t.Fatalf("failed to delete model: %v", err)
	}

	// Verify model.deleted event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeModelDeleted, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for model.deleted event")
	}
}

func TestRegisterHooks_ModelProviderCreatedUpdatedDeleted(t *testing.T) {
	client := setupTestDB(t)
	router := NewEventRouter(10)
	defer router.Close()

	RegisterHooks(client, router)

	ctx := context.Background()

	// Subscribe to all model provider events
	ch, cancel := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"modelprovider.*"}})
	defer cancel()

	// Create provider
	provider, err := client.ModelProvider.Create().
		SetName("test-provider").
		SetProviderType(types.ModelProviderTypeOpenAI).
		SetSecret([]byte("test-secret")).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Verify modelprovider.created event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeModelProviderCreated, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for modelprovider.created event")
	}

	// Update provider
	provider, err = client.ModelProvider.UpdateOne(provider).
		SetName("updated-provider").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to update provider: %v", err)
	}

	// Verify modelprovider.updated event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeModelProviderUpdated, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for modelprovider.updated event")
	}

	// Delete provider
	err = client.ModelProvider.DeleteOne(provider).Exec(ctx)
	if err != nil {
		t.Fatalf("failed to delete provider: %v", err)
	}

	// Verify modelprovider.deleted event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeModelProviderDeleted, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for modelprovider.deleted event")
	}
}

func TestRegisterHooks_TaskCreatedDeleted(t *testing.T) {
	client := setupTestDB(t)
	router := NewEventRouter(10)
	defer router.Close()

	RegisterHooks(client, router)

	ctx := context.Background()

	// Subscribe to all task events
	ch, cancel := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"task.*"}})
	defer cancel()

	// Create agent first (required for task)
	agent, err := client.Agent.Create().
		SetName("test-agent").
		SetDescription("Test").
		SetInstructions("Test").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Create task
	task, err := client.Task.Create().
		SetDescription("Test task").
		SetPhase(types.TaskPhaseAwaiting).
		SetAgent(agent).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Verify task.created event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeTaskCreated, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
		payload, ok := event.Payload.(*TaskEventPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", event.Payload)
		}
		if diff := cmp.Diff(task.ID, payload.Task.ID); diff != "" {
			t.Errorf("task ID mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for task.created event")
	}

	// Delete task
	err = client.Task.DeleteOne(task).Exec(ctx)
	if err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}

	// Verify task.deleted event
	select {
	case event := <-ch:
		if diff := cmp.Diff(EventTypeTaskDeleted, event.Type); diff != "" {
			t.Errorf("event type mismatch (-want +got):\n%s", diff)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for task.deleted event")
	}
}

func TestRegisterHooks_TaskUpdateDoesNotEmitEvent(t *testing.T) {
	client := setupTestDB(t)
	router := NewEventRouter(10)
	defer router.Close()

	RegisterHooks(client, router)

	ctx := context.Background()

	// Create agent first
	agent, err := client.Agent.Create().
		SetName("test-agent").
		SetDescription("Test").
		SetInstructions("Test").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Create task
	task, err := client.Task.Create().
		SetDescription("Test task").
		SetPhase(types.TaskPhaseAwaiting).
		SetAgent(agent).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Subscribe to task.updated events
	ch, cancel := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"task.updated"}})
	defer cancel()

	// Update the task
	_, err = client.Task.UpdateOne(task).
		SetPhase(types.TaskPhaseRunning).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to update task: %v", err)
	}

	// Verify NO task.updated event is emitted (that's from reconciler)
	select {
	case event := <-ch:
		t.Errorf("unexpected task.updated event received: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Expected: no event
	}
}

func TestRegisterHooks_MultipleEventsInSequence(t *testing.T) {
	client := setupTestDB(t)
	router := NewEventRouter(10)
	defer router.Close()

	RegisterHooks(client, router)

	ctx := context.Background()

	// Subscribe to all events
	ch, cancel := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"*"}})
	defer cancel()

	// Create provider
	provider, err := client.ModelProvider.Create().
		SetName("test-provider").
		SetProviderType(types.ModelProviderTypeOpenAI).
		SetSecret([]byte("test-secret")).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Create agent
	agent, err := client.Agent.Create().
		SetName("test-agent").
		SetDescription("Test").
		SetInstructions("Test").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Collect events
	var events []*StreamEvent
	timeout := time.After(time.Second)
	for i := 0; i < 2; i++ {
		select {
		case event := <-ch:
			events = append(events, event)
		case <-timeout:
			break
		}
	}

	// Verify we got both events
	eventTypes := make([]string, len(events))
	for i, e := range events {
		eventTypes[i] = e.Type
	}

	wantTypes := []string{EventTypeModelProviderCreated, EventTypeAgentCreated}
	if diff := cmp.Diff(wantTypes, eventTypes, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
		t.Errorf("event types mismatch (-want +got):\n%s", diff)
	}

	// Cleanup to avoid unused variable warning
	_ = provider
	_ = agent
}
