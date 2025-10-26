package event_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furisto/construct/backend/event"
	"github.com/google/uuid"
)

type TaskStartedEvent struct {
	TaskID uuid.UUID
}

func (TaskStartedEvent) Event() {}

type TaskCompletedEvent struct {
	TaskID   uuid.UUID
	Duration time.Duration
}

func (TaskCompletedEvent) Event() {}

type TaskFailedEvent struct {
	TaskID uuid.UUID
	Error  string
}

func (TaskFailedEvent) Event() {}

func TestEventBus_BasicPublishSubscribe(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)
	defer bus.Close()

	done := make(chan struct{})
	sub := event.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
		close(done)
	}, nil)
	defer sub.Unsubscribe()

	taskID := uuid.New()
	event.Publish(bus, TaskStartedEvent{TaskID: taskID})

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected event to be received")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)
	defer bus.Close()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(5)
	for range 5 {
		sub := event.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
			wg.Done()
		}, nil)
		defer sub.Unsubscribe()
	}
	go func() {
		wg.Wait()
		close(done)
	}()

	event.Publish(bus, TaskStartedEvent{TaskID: uuid.New()})

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected all subscribers to be called")
	}
}

func TestEventBus_DifferentEventTypes(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)
	defer bus.Close()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)
	var startedReceived atomic.Bool
	var completedReceived atomic.Bool
	var failedReceived atomic.Bool

	sub1 := event.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
		defer wg.Done()
		startedReceived.Store(true)
	}, nil)
	defer sub1.Unsubscribe()

	sub2 := event.Subscribe(bus, func(ctx context.Context, e TaskCompletedEvent) {
		defer wg.Done()
		completedReceived.Store(true)
	}, nil)
	defer sub2.Unsubscribe()

	sub3 := event.Subscribe(bus, func(ctx context.Context, e TaskFailedEvent) {
		defer wg.Done()
		failedReceived.Store(true)
	}, nil)
	defer sub3.Unsubscribe()

	taskID := uuid.New()

	event.Publish(bus, TaskStartedEvent{TaskID: taskID})
	event.Publish(bus, TaskCompletedEvent{TaskID: taskID, Duration: time.Second})
	event.Publish(bus, TaskFailedEvent{TaskID: taskID, Error: "test error"})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected all events to be received")
	}

	if !startedReceived.Load() {
		t.Error("TaskStartedEvent was not received")
	}
	if !completedReceived.Load() {
		t.Error("TaskCompletedEvent was not received")
	}
	if !failedReceived.Load() {
		t.Error("TaskFailedEvent was not received")
	}
}

func TestEventBus_ThreadSafety(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)
	defer bus.Close()

	var count atomic.Int32
	var wg sync.WaitGroup

	// Subscribe from multiple goroutines
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
				count.Add(1)
			}, nil)
		}()
	}

	wg.Wait()

	// Publish from multiple goroutines
	numPublishes := 5
	for range numPublishes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event.Publish(bus, TaskStartedEvent{TaskID: uuid.New()})
		}()
	}

	wg.Wait()

	// Wait for all async deliveries
	time.Sleep(200 * time.Millisecond)

	// Should have 10 subscribers * 5 publishes = 50 invocations
	expected := int32(10 * numPublishes)
	if count.Load() != expected {
		t.Errorf("Expected %d handler invocations, got %d", expected, count.Load())
	}
}

func TestEventBus_SubscriberCount(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)
	defer bus.Close()

	if count := event.SubscriberCount[TaskStartedEvent](bus); count != 0 {
		t.Errorf("Expected 0 subscribers initially, got %d", count)
	}

	event.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {}, nil)
	event.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {}, nil)
	event.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {}, nil)

	if count := event.SubscriberCount[TaskStartedEvent](bus); count != 3 {
		t.Errorf("Expected 3 subscribers, got %d", count)
	}

	// Different event type should have 0 subscribers
	if count := event.SubscriberCount[TaskCompletedEvent](bus); count != 0 {
		t.Errorf("Expected 0 subscribers for TaskCompletedEvent, got %d", count)
	}
}

func TestEventBus_Close_DiscardEvents(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)

	var processed atomic.Int32
	numEvents := 1000

	event.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
		time.Sleep(100 * time.Millisecond) // Simulate work
		processed.Add(1)
	}, nil)

	for range numEvents {
		event.Publish(bus, TaskStartedEvent{TaskID: uuid.New()})
	}
	bus.Close()

	// All events should have been processed
	if processed.Load() == int32(numEvents) {
		t.Errorf("Expected events to be discarded, but %d events were processed", processed.Load())
	}
}

func TestEventBus_ChannelSubscription(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)
	defer bus.Close()

	ch, sub := event.SubscribeChannel[TaskStartedEvent](bus, 10, nil)
	defer sub.Unsubscribe()

	taskID := uuid.New()
	event.Publish(bus, TaskStartedEvent{TaskID: taskID})

	select {
	case event := <-ch:
		if event.TaskID != taskID {
			t.Errorf("Expected TaskID %s, got %s", taskID, event.TaskID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event on channel")
	}
}

func TestEventBus_ChannelSubscriptionMultipleEvents(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)
	defer bus.Close()

	ch, sub := event.SubscribeChannel[TaskStartedEvent](bus, 10, nil)
	defer sub.Unsubscribe()

	numEvents := 5
	taskIDs := make([]uuid.UUID, numEvents)
	for i := range numEvents {
		taskIDs[i] = uuid.New()
		event.Publish(bus, TaskStartedEvent{TaskID: taskIDs[i]})
	}

	receivedIDs := make(map[uuid.UUID]bool)
	for range numEvents {
		select {
		case event := <-ch:
			receivedIDs[event.TaskID] = true
		case <-time.After(100 * time.Millisecond):
			t.Error("Timeout waiting for event on channel")
		}
	}

	for _, id := range taskIDs {
		if !receivedIDs[id] {
			t.Errorf("TaskID %s was not received", id)
		}
	}
}

func TestEventBus_ChannelSubscriptionUnsubscribe(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)
	defer bus.Close()

	ch, sub := event.SubscribeChannel[TaskStartedEvent](bus, 10, nil)

	taskID1 := uuid.New()
	event.Publish(bus, TaskStartedEvent{TaskID: taskID1})

	select {
	case event := <-ch:
		if event.TaskID != taskID1 {
			t.Errorf("Expected TaskID %s, got %s", taskID1, event.TaskID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for first event")
	}

	sub.Unsubscribe()

	taskID2 := uuid.New()
	event.Publish(bus, TaskStartedEvent{TaskID: taskID2})

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Expected channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for channel to be closed")
	}
}

func TestEventBus_ChannelAndHandlerMixed(t *testing.T) {
	t.Parallel()

	bus := event.NewBus(nil)
	defer bus.Close()

	done := make(chan struct{})
	event.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
		close(done)
	}, nil)

	ch, sub := event.SubscribeChannel[TaskStartedEvent](bus, 10, nil)
	defer sub.Unsubscribe()

	taskID := uuid.New()
	event.Publish(bus, TaskStartedEvent{TaskID: taskID})

	// Both handler and channel should receive the event
	select {
	case event := <-ch:
		if event.TaskID != taskID {
			t.Errorf("Expected TaskID %s, got %s", taskID, event.TaskID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event on channel")
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for handler to be called")
	}
}

// Mock database for testing MessageStreamProvider
type MockMessageDB struct {
	messages []MockMessage
}

type MockMessage struct {
	ID            uuid.UUID
	TaskID        uuid.UUID
	Content       string
	ProcessedTime *time.Time
}

func (db *MockMessageDB) GetMessagesForTask(taskID uuid.UUID) []MockMessage {
	var result []MockMessage
	for _, msg := range db.messages {
		if msg.TaskID == taskID && msg.ProcessedTime != nil {
			result = append(result, msg)
		}
	}
	return result
}
