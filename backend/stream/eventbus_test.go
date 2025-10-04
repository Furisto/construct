package stream_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furisto/construct/backend/stream"
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

	bus := stream.NewBus(nil)
	defer bus.Close()

	done := make(chan struct{})
	sub := stream.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
		close(done)
	})
	defer sub.Unsubscribe()

	taskID := uuid.New()
	stream.Publish(bus, TaskStartedEvent{TaskID: taskID})

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected event to be received")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	t.Parallel()

	bus := stream.NewBus(nil)
	defer bus.Close()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(5)
	for range 5 {
		sub := stream.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
			wg.Done()
		})
		defer sub.Unsubscribe()
	}
	go func() {
		wg.Wait()
		close(done)
	}()

	stream.Publish(bus, TaskStartedEvent{TaskID: uuid.New()})

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected all subscribers to be called")
	}
}

func TestEventBus_DifferentEventTypes(t *testing.T) {
	t.Parallel()

	bus := stream.NewBus(nil)
	defer bus.Close()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)
	var startedReceived atomic.Bool
	var completedReceived atomic.Bool
	var failedReceived atomic.Bool

	sub1 := stream.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
		defer wg.Done()
		startedReceived.Store(true)
	})
	defer sub1.Unsubscribe()

	sub2 := stream.Subscribe(bus, func(ctx context.Context, e TaskCompletedEvent) {
		defer wg.Done()
		completedReceived.Store(true)
	})
	defer sub2.Unsubscribe()

	sub3 := stream.Subscribe(bus, func(ctx context.Context, e TaskFailedEvent) {
		defer wg.Done()
		failedReceived.Store(true)
	})
	defer sub3.Unsubscribe()

	taskID := uuid.New()

	stream.Publish(bus, TaskStartedEvent{TaskID: taskID})
	stream.Publish(bus, TaskCompletedEvent{TaskID: taskID, Duration: time.Second})
	stream.Publish(bus, TaskFailedEvent{TaskID: taskID, Error: "test error"})

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

	bus := stream.NewBus(nil)
	defer bus.Close()

	var count atomic.Int32
	var wg sync.WaitGroup

	// Subscribe from multiple goroutines
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
				count.Add(1)
			})
		}()
	}

	wg.Wait()

	// Publish from multiple goroutines
	numPublishes := 5
	for range numPublishes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream.Publish(bus, TaskStartedEvent{TaskID: uuid.New()})
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

	bus := stream.NewBus(nil)
	defer bus.Close()

	if count := stream.SubscriberCount[TaskStartedEvent](bus); count != 0 {
		t.Errorf("Expected 0 subscribers initially, got %d", count)
	}

	stream.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {})
	stream.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {})
	stream.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {})

	if count := stream.SubscriberCount[TaskStartedEvent](bus); count != 3 {
		t.Errorf("Expected 3 subscribers, got %d", count)
	}

	// Different event type should have 0 subscribers
	if count := stream.SubscriberCount[TaskCompletedEvent](bus); count != 0 {
		t.Errorf("Expected 0 subscribers for TaskCompletedEvent, got %d", count)
	}
}

func TestEventBus_Close_DiscardEvents(t *testing.T) {
	t.Parallel()

	bus := stream.NewBus(nil)

	var processed atomic.Int32
	numEvents := 1000

	stream.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
		time.Sleep(100 * time.Millisecond) // Simulate work
		processed.Add(1)
	})

	for range numEvents {
		stream.Publish(bus, TaskStartedEvent{TaskID: uuid.New()})
	}
	bus.Close()

	// All events should have been processed
	if processed.Load() == int32(numEvents) {
		t.Errorf("Expected events to be discarded, but %d events were processed", processed.Load())
	}
}

func TestEventBus_ChannelSubscription(t *testing.T) {
	t.Parallel()

	bus := stream.NewBus(nil)
	defer bus.Close()

	ch, sub := stream.SubscribeChannel[TaskStartedEvent](bus, 10)
	defer sub.Unsubscribe()

	taskID := uuid.New()
	stream.Publish(bus, TaskStartedEvent{TaskID: taskID})

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

	bus := stream.NewBus(nil)
	defer bus.Close()

	ch, sub := stream.SubscribeChannel[TaskStartedEvent](bus, 10)
	defer sub.Unsubscribe()

	numEvents := 5
	taskIDs := make([]uuid.UUID, numEvents)
	for i := range numEvents {
		taskIDs[i] = uuid.New()
		stream.Publish(bus, TaskStartedEvent{TaskID: taskIDs[i]})
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

	bus := stream.NewBus(nil)
	defer bus.Close()

	ch, sub := stream.SubscribeChannel[TaskStartedEvent](bus, 10)

	taskID1 := uuid.New()
	stream.Publish(bus, TaskStartedEvent{TaskID: taskID1})

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
	stream.Publish(bus, TaskStartedEvent{TaskID: taskID2})

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
	
	bus := stream.NewBus(nil)
	defer bus.Close()

	done := make(chan struct{})
	stream.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
		close(done)
	})

	ch, sub := stream.SubscribeChannel[TaskStartedEvent](bus, 10)
	defer sub.Unsubscribe()

	taskID := uuid.New()
	stream.Publish(bus, TaskStartedEvent{TaskID: taskID})

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
