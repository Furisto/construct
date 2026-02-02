package event

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultChannelBufferSize is the default buffer size for subscriber channels.
	DefaultChannelBufferSize = 100
)

// StreamEvent is the domain event type used by the EventRouter.
// It contains the event metadata and payload for internal routing.
type StreamEvent struct {
	// Type is the event type string (e.g., "task.created", "message.chunk", "tool.called").
	Type string

	// Action is the action that occurred (created, updated, deleted, or empty for non-CRUD events).
	Action string

	// Timestamp is when the change occurred.
	Timestamp time.Time

	// TaskID is the optional task scope for filtering. Nil for non-task-scoped events.
	TaskID *uuid.UUID

	// Payload is the domain payload (e.g., *memory.Task, *ToolCallPayload).
	Payload any
}

// SubscribeOptions configures event eventSubscription filtering.
type SubscribeOptions struct {
	// EventTypes specifies which event types to receive using glob patterns.
	// Supports: "*" (all), "entity.*" (e.g., "task.*"), "*.action" (e.g., "*.created"), or exact match.
	// Empty slice subscribes to all events.
	// Note: internal.* events are filtered out unless Internal is set to true.
	EventTypes []string

	// TaskID filters events to only those related to a specific task.
	// When set, only task-scoped events for this task are delivered.
	TaskID string

	// ReplayAfterMessageID enables replay of message.created events after this message ID.
	// Only applicable when TaskID is also set.
	ReplayAfterMessageID string

	// Internal allows subscribing to internal.* events.
	// This should only be set by internal components, not external API consumers.
	Internal bool
}

// eventSubscription represents an active event eventSubscription.
type eventSubscription struct {
	id         uuid.UUID
	patterns   []string
	taskID     *uuid.UUID
	channel    chan *StreamEvent
	cancelFunc context.CancelFunc
}

// EventRouter manages event eventSubscriptions and distribution.
type EventRouter struct {
	eventSubscriptions map[uuid.UUID]*eventSubscription
	mu            sync.RWMutex
	bufferSize    int
	closed        bool
}

// NewEventRouter creates a new EventRouter with the specified channel buffer size.
func NewEventRouter(bufferSize int) *EventRouter {
	if bufferSize <= 0 {
		bufferSize = DefaultChannelBufferSize
	}
	return &EventRouter{
		eventSubscriptions: make(map[uuid.UUID]*eventSubscription),
		bufferSize:    bufferSize,
	}
}

// Subscribe creates a new eventSubscription with pattern matching and returns a channel for receiving events.
// Call the returned cancel function to unsubscribe and close the channel.
// The channel is also closed if ctx is cancelled.
func (r *EventRouter) Subscribe(ctx context.Context, opts SubscribeOptions) (<-chan *StreamEvent, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		ch := make(chan *StreamEvent)
		close(ch)
		return ch, func() {}
	}

	// Parse patterns, filtering out internal event patterns for external consumers
	patterns := opts.EventTypes
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}
	if !opts.Internal {
		patterns = filterInternalPatterns(patterns)
	}

	// Parse task ID
	var taskID *uuid.UUID
	if opts.TaskID != "" {
		parsed, err := uuid.Parse(opts.TaskID)
		if err == nil {
			taskID = &parsed
		}
	}

	subCtx, cancel := context.WithCancel(ctx)
	ch := make(chan *StreamEvent, r.bufferSize)

	sub := &eventSubscription{
		id:         uuid.New(),
		patterns:   patterns,
		taskID:     taskID,
		channel:    ch,
		cancelFunc: cancel,
	}

	r.eventSubscriptions[sub.id] = sub

	// Cleanup goroutine
	go func() {
		<-subCtx.Done()
		r.unsubscribe(sub.id)
	}()

	return ch, cancel
}

// unsubscribe removes a eventSubscription and closes its channel.
func (r *EventRouter) unsubscribe(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sub, ok := r.eventSubscriptions[id]; ok {
		close(sub.channel)
		delete(r.eventSubscriptions, id)
	}
}

// Publish sends an event to all matching subscribers.
// Events are delivered non-blocking; if a subscriber's channel is full, the event is dropped.
func (r *EventRouter) Publish(event *StreamEvent) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return
	}

	for _, sub := range r.eventSubscriptions {
		if r.matches(sub, event) {
			select {
			case sub.channel <- event:
				// Delivered
			default:
				// Channel full, drop event
				slog.Debug("dropped event due to full channel buffer",
					"event_type", event.Type,
					"eventSubscription_id", sub.id,
				)
			}
		}
	}
}

// matches checks if an event matches a eventSubscription's filters.
func (r *EventRouter) matches(sub *eventSubscription, event *StreamEvent) bool {
	// Check task scope filter
	if sub.taskID != nil {
		// Subscriber wants task-scoped events only
		if event.TaskID == nil || *event.TaskID != *sub.taskID {
			return false
		}
	}

	// Check pattern filter
	for _, pattern := range sub.patterns {
		if matchPattern(pattern, event.Type) {
			return true
		}
	}

	return false
}

// internalEventPrefix is the prefix for internal coordination events.
const internalEventPrefix = "internal."

// filterInternalPatterns removes patterns that would match internal events.
func filterInternalPatterns(patterns []string) []string {
	filtered := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if !strings.HasPrefix(p, internalEventPrefix) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// isInternalEvent checks if an event type is an internal event.
func isInternalEvent(eventType string) bool {
	return strings.HasPrefix(eventType, internalEventPrefix)
}

// matchPattern checks if an event type matches a glob pattern.
// Supported patterns:
//   - "*" matches all event types (except internal.* events)
//   - "entity.*" matches all events for that entity (e.g., "task.*" matches "task.created")
//   - "*.action" matches that action across all entities (e.g., "*.created" matches "task.created")
//   - Exact strings match exactly (e.g., "message.chunk")
//
// Note: Internal events (internal.*) are only matched by explicit internal.* patterns,
// never by wildcards. This prevents external consumers from accidentally receiving internal events.
func matchPattern(pattern, eventType string) bool {
	// Internal events require explicit internal.* patterns to match
	if isInternalEvent(eventType) && !strings.HasPrefix(pattern, internalEventPrefix) {
		return false
	}

	if pattern == "*" {
		return true
	}

	if pattern == eventType {
		return true
	}

	// Split pattern and event type
	patternParts := strings.SplitN(pattern, ".", 2)
	eventParts := strings.SplitN(eventType, ".", 2)

	if len(patternParts) != 2 || len(eventParts) != 2 {
		return false
	}

	patternEntity, patternAction := patternParts[0], patternParts[1]
	eventEntity, eventAction := eventParts[0], eventParts[1]

	// "entity.*" pattern
	if patternAction == "*" && patternEntity == eventEntity {
		return true
	}

	// "*.action" pattern
	if patternEntity == "*" && patternAction == eventAction {
		return true
	}

	return false
}

// Close shuts down the router and closes all eventSubscription channels.
func (r *EventRouter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	r.closed = true

	for id, sub := range r.eventSubscriptions {
		sub.cancelFunc()
		close(sub.channel)
		delete(r.eventSubscriptions, id)
	}
}

// SubscriptionCount returns the number of active eventSubscriptions.
// Useful for testing and debugging.
func (r *EventRouter) SubscriptionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.eventSubscriptions)
}
