package event

import (
	"context"
	"log/slog"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// Event is a marker interface that all events must implement.
// This ensures type safety at compile time for event types.
type Event[T any] interface {
	Event()
}

// Handler is a function type that handles events of type T.
// Handlers do not return errors and are executed asynchronously.
type Handler[T any] func(context.Context, T)

type EventFilter[T any] func(T) bool

type Bus struct {
	ctx         context.Context
	cancel      context.CancelFunc
	subscribers map[reflect.Type][]subscriber
	mu          sync.RWMutex
	wg          sync.WaitGroup
	closed      atomic.Bool

	workQueue chan workItem

	metrics *eventBusMetricsProvider
}

type workItem struct {
	event     any
	eventType string
	invoke    func(context.Context, any)
}

// subscriber represents either a handler function or a channel subscriber
type subscriber struct {
	id      uuid.UUID
	invoke  func(context.Context, any)
	channel any
}

type Subscription struct {
	bus       *Bus
	eventType reflect.Type
	id        uuid.UUID
	once      sync.Once
}

func NewBus(metricsRegistry *prometheus.Registry) *Bus {
	ctx, cancel := context.WithCancel(context.Background())

	bus := &Bus{
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[reflect.Type][]subscriber),
		workQueue:   make(chan workItem, 100*10),
		metrics:     newEventBusMetricsProvider(metricsRegistry),
	}

	for range 100 {
		bus.wg.Add(1)
		go bus.worker()
	}

	return bus
}

// worker processes events from the work queue
func (bus *Bus) worker() {
	defer bus.wg.Done()

	for {
		select {
		case <-bus.ctx.Done():
			return
		case item := <-bus.workQueue:
			bus.processWorkItem(item)
		}
	}
}

// processWorkItem handles a single event delivery
func (bus *Bus) processWorkItem(item workItem) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(bus.ctx, "panic in processWorkItem",
				"error", r,
				"event_type", item.eventType,
				"stack", string(debug.Stack()),
			)
		}
	}()

	item.invoke(bus.ctx, item.event)
	bus.metrics.IncrementDelivered(item.eventType)
}

// Subscribe registers a handler for events of type T. Returns a Subscription
// that can be used to unsubscribe. The handler will be called asynchronously
// whenever an event of type T is published.
//
// Example:
//
//	sub := Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
//	    log.Printf("Task started: %s", e.TaskID)
//	})
//	defer sub.Unsubscribe()
func Subscribe[T Event[T]](bus *Bus, handler Handler[T], filter EventFilter[T]) *Subscription {
	if bus.closed.Load() {
		slog.WarnContext(bus.ctx, "attempted to subscribe to closed event bus")
		return &Subscription{bus: bus}
	}

	var zero T
	eventType := reflect.TypeOf(zero)

	if filter == nil {
		filter = func(event T) bool {
			return true
		}
	}

	id := uuid.New()
	sub := subscriber{
		id: id,
		invoke: func(ctx context.Context, event any) {
			if typedEvent, ok := event.(T); ok {
				if filter(typedEvent) {
					handler(ctx, typedEvent)
				}
			}
		},
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.subscribers[eventType] = append(bus.subscribers[eventType], sub)

	return &Subscription{
		bus:       bus,
		eventType: eventType,
		id:        id,
	}
}

// SubscribeChannel creates a channel subscription for events of type T.
// Returns a receive-only channel that will receive all events of type T,
// and a Subscription that can be used to unsubscribe.
//
// The channel has a buffer size specified by bufferSize. If the buffer is full,
// events will be dropped to prevent blocking publishers.
//
// The consumer should call Unsubscribe() when done to clean up resources and
// close the channel.
//
// Example:
//
//	ch, sub := SubscribeChannel[TaskStartedEvent](bus, 10)
//	defer sub.Unsubscribe()
//	for event := range ch {
//	    log.Printf("Task started: %s", event.TaskID)
//	}
func SubscribeChannel[T Event[T]](bus *Bus, bufferSize int, filter EventFilter[T]) (<-chan T, *Subscription) {
	if bus.closed.Load() {
		slog.WarnContext(bus.ctx, "attempted to subscribe channel to closed event bus")
		ch := make(chan T)
		close(ch)
		return ch, &Subscription{bus: bus}
	}

	var zero T
	eventType := reflect.TypeOf(zero)
	eventTypeName := eventType.String()

	ch := make(chan T, bufferSize)
	id := uuid.New()

	if filter == nil {
		filter = func(event T) bool {
			return true
		}
	}

	sub := subscriber{
		id:      id,
		channel: ch,
		invoke: func(ctx context.Context, event any) {
			if typedEvent, ok := event.(T); ok {
				if filter(typedEvent) {
					select {
					case ch <- typedEvent:
					default:
						// Drop event if channel is full
						bus.metrics.IncrementDropped(eventTypeName)
						slog.DebugContext(ctx, "dropped event due to full channel buffer",
							"event_type", eventTypeName,
							"subscriber_id", id,
						)
					}
				}
			}
		},
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.subscribers[eventType] = append(bus.subscribers[eventType], sub)

	return ch, &Subscription{
		bus:       bus,
		eventType: eventType,
		id:        id,
	}
}

// Unsubscribe removes the subscription. For channel subscriptions, it also closes
// the channel. Safe to call multiple times.
func (s *Subscription) Unsubscribe() {
	s.once.Do(func() {
		s.bus.mu.Lock()
		defer s.bus.mu.Unlock()

		if s.bus.closed.Load() {
			return
		}

		subscribers := s.bus.subscribers[s.eventType]
		for i, sub := range subscribers {
			if sub.id == s.id {
				s.bus.subscribers[s.eventType] = append(subscribers[:i], subscribers[i+1:]...)

				// Close the channel if this is a channel subscription
				if sub.channel != nil {
					chVal := reflect.ValueOf(sub.channel)
					chVal.Close()
				}

				break
			}
		}
	})
}

// Publish publishes an event to all registered handlers for that event type.
// Events are queued and delivered asynchronously by worker goroutines.
// The provided context is passed to handlers and can be used for cancellation.
//
// Example:
//
//	Publish(bus, ctx, TaskStartedEvent{TaskID: taskID})
func Publish[T Event[T]](bus *Bus, event T) {
	if bus.closed.Load() {
		slog.DebugContext(bus.ctx, "attempted to publish to closed event bus")
		return
	}

	eventType := reflect.TypeOf(event)
	eventTypeName := eventType.String()

	bus.mu.RLock()
	subs := bus.subscribers[eventType]
	// Create a copy of subscribers to avoid holding the lock during invocation
	subsCopy := make([]subscriber, len(subs))
	copy(subsCopy, subs)
	bus.mu.RUnlock()

	for _, sub := range subsCopy {
		item := workItem{
			event:     event,
			eventType: eventTypeName,
			invoke:    sub.invoke,
		}

		select {
		case bus.workQueue <- item:
			// Successfully queued
		case <-bus.ctx.Done():
			// Bus is closing
			return
		default:
			// Drop event if queue is full
			bus.metrics.IncrementDropped(eventTypeName)
			slog.DebugContext(bus.ctx, "dropped event due to full work queue",
				"event_type", eventTypeName,
			)
		}
	}

	bus.metrics.IncrementPublished(eventTypeName)
}

// Close gracefully shuts down the EventBus, cancels its context, closes all
// channel subscriptions, and waits for all in-flight event deliveries to complete.
// After Close is called, no new events should be published. Safe to call multiple times.
func (bus *Bus) Close() {
	if !bus.closed.CompareAndSwap(false, true) {
		return
	}

	slog.DebugContext(bus.ctx, "closing event bus")

	// Cancel context to stop workers
	bus.cancel()
	close(bus.workQueue)

	// Wait for all workers to finish
	bus.wg.Wait()

	// Close all channel subscriptions
	bus.mu.Lock()
	defer bus.mu.Unlock()
	for eventType, subs := range bus.subscribers {
		for _, sub := range subs {
			if sub.channel != nil {
				chVal := reflect.ValueOf(sub.channel)
				chVal.Close()
			}
		}
		delete(bus.subscribers, eventType)
	}

	slog.DebugContext(bus.ctx, "event bus closed")
}

func (bus *Bus) IsClosed() bool {
	return bus.closed.Load()
}

// SubscriberCount returns the number of subscribers for a given event type.
// This is primarily useful for testing and debugging.
func SubscriberCount[T Event[T]](bus *Bus) int {
	var zero T
	eventType := reflect.TypeOf(zero)

	bus.mu.RLock()
	defer bus.mu.RUnlock()

	return len(bus.subscribers[eventType])
}

type MessageCreatedEvent struct {
	MessageID uuid.UUID
	TaskID    uuid.UUID
}

func (MessageCreatedEvent) Event() {}
