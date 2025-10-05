# Event Bus

A production-grade, strongly-typed event bus implementation for Go using generics.

## Features

### Core Functionality
- **Strongly-typed events** - Type-safe event publishing and subscription using Go generics
- **Multiple subscription modes** - Function handlers or channel-based iteration
- **Asynchronous delivery** - Non-blocking event delivery with no guaranteed order
- **Thread-safe** - Safe concurrent access from multiple goroutines

### Production Features
- **Bounded concurrency** - Worker pool limits concurrent event processing (default: 100 workers)
- **Structured logging** - Uses `log/slog` for observability (panic recovery, dropped events)
- **Prometheus metrics** - Tracks published, delivered, dropped events, panics, and subscriber counts
- **Graceful shutdown** - Waits for in-flight events before closing
- **Panic recovery** - Handlers that panic don't crash the bus or affect other subscribers
- **Resource management** - Guards against use-after-close, supports unsubscribe for all subscription types

## Usage

### Basic Example

```go
import "github.com/furisto/construct/backend/agent"

// Define event type
type TaskStartedEvent struct {
    TaskID uuid.UUID
}
func (TaskStartedEvent) Event() {}

// Create bus
ctx := context.Background()
bus := agent.NewEventBus(ctx)
defer bus.Close()

// Subscribe with handler
sub := agent.Subscribe(bus, func(ctx context.Context, e TaskStartedEvent) {
    log.Printf("Task started: %s", e.TaskID)
})
defer sub.Unsubscribe()

// Publish event
agent.Publish(bus, ctx, TaskStartedEvent{TaskID: taskID})
```

### Channel Subscription

```go
// Subscribe with channel
ch, sub := agent.SubscribeChannel[TaskStartedEvent](bus, 10)
defer sub.Unsubscribe()

// Iterate over events
for event := range ch {
    log.Printf("Task started: %s", event.TaskID)
}
```

### Production Configuration

```go
bus := agent.NewEventBusWithOptions(ctx, agent.EventBusOptions{
    MaxWorkers:      50,                    // Concurrent worker limit
    Logger:          slog.Default(),        // Structured logger
    MetricsRegistry: prometheusRegistry,    // Metrics collection
})
defer bus.Close()
```

## Metrics

When configured with a Prometheus registry, the following metrics are collected:

- `eventbus_events_published_total` - Total events published by type
- `eventbus_events_delivered_total` - Total events delivered by type
- `eventbus_events_dropped_total` - Events dropped due to full channel buffers
- `eventbus_handler_panics_total` - Handler panics by event type
- `eventbus_subscribers` - Current subscriber count by event type

## Design Decisions

1. **Worker Pool** - Prevents goroutine explosion under high load by queueing work items
2. **No Error Returns** - Handlers are fire-and-forget; use logging for observability
3. **Drop on Full** - Channel subscriptions drop events when buffer is full to prevent blocking
4. **Type Safety** - `Event[T]` interface ensures compile-time type checking
5. **Graceful Degradation** - Operations after close are safe (no-ops with logging)

## Testing

Run tests with race detector:
```bash
go test -race ./backend/agent/...
```

Run benchmarks:
```bash
go test -bench=. ./backend/agent/...
```

## See Also

- `eventbus_test.go` - Comprehensive test suite
- `eventbus_example_test.go` - Usage examples
