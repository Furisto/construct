package event

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		eventType string
		want      bool
	}{
		// Wildcard matches all
		{"wildcard matches task.created", "*", "task.created", true},
		{"wildcard matches message.chunk", "*", "message.chunk", true},
		{"wildcard matches tool.called", "*", "tool.called", true},

		// Entity wildcard
		{"task.* matches task.created", "task.*", "task.created", true},
		{"task.* matches task.updated", "task.*", "task.updated", true},
		{"task.* matches task.deleted", "task.*", "task.deleted", true},
		{"task.* does not match message.created", "task.*", "message.created", false},
		{"message.* matches message.chunk", "message.*", "message.chunk", true},

		// Action wildcard
		{"*.created matches task.created", "*.created", "task.created", true},
		{"*.created matches agent.created", "*.created", "agent.created", true},
		{"*.created does not match task.updated", "*.created", "task.updated", false},

		// Exact match
		{"exact match task.created", "task.created", "task.created", true},
		{"exact match message.chunk", "message.chunk", "message.chunk", true},
		{"exact no match", "task.created", "task.updated", false},

		// Edge cases
		{"empty pattern", "", "task.created", false},
		{"single part pattern", "task", "task.created", false},
		{"single part event", "task.*", "task", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPattern(tt.pattern, tt.eventType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEventRouter_Subscribe(t *testing.T) {
	router := NewEventRouter(10)
	defer router.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsubscribe := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"task.*"},
	})
	defer unsubscribe()

	assert.NotNil(t, ch)
	assert.Equal(t, 1, router.SubscriptionCount())
}

func TestEventRouter_Unsubscribe(t *testing.T) {
	router := NewEventRouter(10)
	defer router.Close()

	ctx := context.Background()
	_, unsubscribe := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"*"},
	})

	assert.Equal(t, 1, router.SubscriptionCount())

	unsubscribe()

	// Give cleanup goroutine time to run
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, 0, router.SubscriptionCount())
}

func TestEventRouter_Publish(t *testing.T) {
	router := NewEventRouter(10)
	defer router.Close()

	ctx := context.Background()
	ch, unsubscribe := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"task.*"},
	})
	defer unsubscribe()

	taskID := uuid.New()
	event := &StreamEvent{
		Type:      "task.created",
		Action:    "created",
		Timestamp: time.Now(),
		TaskID:    &taskID,
		Payload:   "test payload",
	}

	router.Publish(event)

	select {
	case received := <-ch:
		assert.Equal(t, event.Type, received.Type)
		assert.Equal(t, event.Action, received.Action)
		assert.Equal(t, event.Payload, received.Payload)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventRouter_PublishFiltersNonMatchingPatterns(t *testing.T) {
	router := NewEventRouter(10)
	defer router.Close()

	ctx := context.Background()
	ch, unsubscribe := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"task.*"},
	})
	defer unsubscribe()

	// Publish a message event (should not match task.*)
	event := &StreamEvent{
		Type:      "message.created",
		Action:    "created",
		Timestamp: time.Now(),
		Payload:   "test payload",
	}

	router.Publish(event)

	select {
	case <-ch:
		t.Fatal("should not receive non-matching event")
	case <-time.After(50 * time.Millisecond):
		// Expected: no event received
	}
}

func TestEventRouter_TaskScopeFilter(t *testing.T) {
	router := NewEventRouter(10)
	defer router.Close()

	ctx := context.Background()
	taskID := uuid.New()
	otherTaskID := uuid.New()

	ch, unsubscribe := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"*"},
		TaskID:     taskID.String(),
	})
	defer unsubscribe()

	// Publish event for the subscribed task
	event1 := &StreamEvent{
		Type:      "message.created",
		Action:    "created",
		Timestamp: time.Now(),
		TaskID:    &taskID,
		Payload:   "correct task",
	}
	router.Publish(event1)

	// Publish event for a different task
	event2 := &StreamEvent{
		Type:      "message.created",
		Action:    "created",
		Timestamp: time.Now(),
		TaskID:    &otherTaskID,
		Payload:   "wrong task",
	}
	router.Publish(event2)

	// Publish event with no task scope
	event3 := &StreamEvent{
		Type:      "agent.created",
		Action:    "created",
		Timestamp: time.Now(),
		TaskID:    nil,
		Payload:   "no task",
	}
	router.Publish(event3)

	// Should only receive the first event
	select {
	case received := <-ch:
		assert.Equal(t, "correct task", received.Payload)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}

	// Should not receive more events
	select {
	case received := <-ch:
		t.Fatalf("should not receive more events, got: %v", received.Payload)
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestEventRouter_MultipleSubscribers(t *testing.T) {
	router := NewEventRouter(10)
	defer router.Close()

	ctx := context.Background()

	ch1, unsubscribe1 := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"task.*"},
	})
	defer unsubscribe1()

	ch2, unsubscribe2 := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"*.created"},
	})
	defer unsubscribe2()

	ch3, unsubscribe3 := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"message.*"},
	})
	defer unsubscribe3()

	assert.Equal(t, 3, router.SubscriptionCount())

	// Publish task.created - should match ch1 and ch2
	event := &StreamEvent{
		Type:      "task.created",
		Action:    "created",
		Timestamp: time.Now(),
	}
	router.Publish(event)

	// ch1 should receive (task.*)
	select {
	case <-ch1:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch1 should receive event")
	}

	// ch2 should receive (*.created)
	select {
	case <-ch2:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ch2 should receive event")
	}

	// ch3 should NOT receive (message.*)
	select {
	case <-ch3:
		t.Fatal("ch3 should not receive event")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestEventRouter_ContextCancellation(t *testing.T) {
	router := NewEventRouter(10)
	defer router.Close()

	ctx, cancel := context.WithCancel(context.Background())

	ch, _ := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"*"},
	})

	assert.Equal(t, 1, router.SubscriptionCount())

	// Cancel the context
	cancel()

	// Give cleanup goroutine time to run
	time.Sleep(50 * time.Millisecond)

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed")

	assert.Equal(t, 0, router.SubscriptionCount())
}

func TestEventRouter_Close(t *testing.T) {
	router := NewEventRouter(10)

	ctx := context.Background()
	ch1, _ := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"*"}})
	ch2, _ := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"task.*"}})

	assert.Equal(t, 2, router.SubscriptionCount())

	router.Close()

	// All channels should be closed
	_, ok1 := <-ch1
	_, ok2 := <-ch2
	assert.False(t, ok1, "ch1 should be closed")
	assert.False(t, ok2, "ch2 should be closed")

	assert.Equal(t, 0, router.SubscriptionCount())

	// Publish after close should not panic
	router.Publish(&StreamEvent{Type: "test.event"})

	// Subscribe after close should return closed channel
	ch3, _ := router.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"*"}})
	_, ok3 := <-ch3
	assert.False(t, ok3, "ch3 should be closed immediately")
}

func TestEventRouter_EmptyPatternsSubscribesToAll(t *testing.T) {
	router := NewEventRouter(10)
	defer router.Close()

	ctx := context.Background()
	ch, unsubscribe := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{}, // Empty = all
	})
	defer unsubscribe()

	events := []string{"task.created", "message.chunk", "agent.deleted", "tool.called"}
	for _, eventType := range events {
		router.Publish(&StreamEvent{Type: eventType})
	}

	// Should receive all events
	for i := 0; i < len(events); i++ {
		select {
		case <-ch:
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("should receive event %d", i)
		}
	}
}

func TestEventRouter_DropsEventsOnFullChannel(t *testing.T) {
	router := NewEventRouter(2) // Small buffer
	defer router.Close()

	ctx := context.Background()
	ch, unsubscribe := router.Subscribe(ctx, SubscribeOptions{
		EventTypes: []string{"*"},
	})
	defer unsubscribe()

	// Publish more events than buffer can hold without consuming
	for i := 0; i < 10; i++ {
		router.Publish(&StreamEvent{Type: "test.event", Payload: i})
	}

	// Should only receive buffer size events
	received := 0
	for {
		select {
		case <-ch:
			received++
		case <-time.After(50 * time.Millisecond):
			// No more events
			require.LessOrEqual(t, received, 2, "should not receive more than buffer size")
			return
		}
	}
}
