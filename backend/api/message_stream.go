package api

import (
	"context"
	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/message"
	"github.com/furisto/construct/shared/conv"
	"github.com/google/uuid"
	"log/slog"
)

// MessageStreamProvider wraps the generic EventBus and provides message-specific
// streaming functionality with automatic replay of historical messages.
type MessageStreamProvider struct {
	*event.Bus
	db *memory.Client
}

// NewMessageStreamProvider creates a new MessageStreamProvider that combines
// the generic event bus with message-specific replay capabilities.
func NewMessageStreamProvider(bus *event.Bus, db *memory.Client) *MessageStreamProvider {
	return &MessageStreamProvider{
		Bus: bus,
		db:  db,
	}
}

// Stream subscribes to all messages for a specific task, automatically replaying
// historical messages first, then delivering live messages as they arrive.
//
// The method will:
// 1. Subscribe to live MessageCreatedEvent events filtered by taskID
// 2. Asynchronously replay all historical messages for the task in chronological order
// 3. Continue delivering live messages after replay completes
//
// Example:
//
//	messageStream := NewMessageStreamProvider(bus, db)
//	sub := messageStream.Stream(taskID, func(ctx context.Context, msg MessageCreatedEvent) {
//	    fmt.Printf("Message: %s for task %s", msg.Content, msg.TaskID)
//	})
//	defer sub.Unsubscribe()
func (msp *MessageStreamProvider) Stream(taskID uuid.UUID) (<-chan event.MessageEvent, *event.Subscription) {
	// Create filter that only matches this task
	taskFilter := func(event event.MessageEvent) bool {
		return event.TaskID == taskID
	}

	// Subscribe with filter for live events
	ch, sub := event.SubscribeChannel(msp.Bus, 10, taskFilter)

	// Replay historical messages in background
	go msp.replayTaskMessages(taskID, ch)

	return ch, sub
}

// replayTaskMessages fetches and replays all historical messages for a specific task
// in chronological order. This method runs asynchronously and delivers historical
// messages through the same handler used for live events.
func (msp *MessageStreamProvider) replayTaskMessages(taskID uuid.UUID, ch <-chan event.MessageEvent) {
	ctx := context.Background()

	slog.DebugContext(ctx, "starting message replay for task",
		"task_id", taskID,
	)

	// Query database for historical messages for this task
	messages, err := msp.db.Message.Query().
		Where(message.TaskIDEQ(taskID), message.ProcessedTimeNotNil()).
		Order(message.ByProcessedTime(), memory.Asc()).
		All(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to replay task messages",
			"error", err,
			"task_id", taskID,
		)
		return
	}

	slog.DebugContext(ctx, "replaying historical messages",
		"task_id", taskID,
		"message_count", len(messages),
	)

	// Convert database messages to events and deliver them
	for _, msg := range messages {
		// Check if the bus is still active
		if msp.Bus.IsClosed() {
			slog.DebugContext(ctx, "bus closed during replay, stopping",
				"task_id", taskID,
			)
			return
		}

		// Convert memory message to proto message first
		_, err := conv.ConvertMemoryMessageToProto(msg)
		if err != nil {
			slog.ErrorContext(ctx, "failed to convert message during replay",
				"error", err,
				"message_id", msg.ID,
				"task_id", taskID,
			)
			continue
		}

		// Create event from the message
		evt := event.MessageEvent{
			MessageID: msg.ID,
			TaskID:    msg.TaskID,
		}

		// Deliver the historical event
		event.Publish(msp.Bus, evt)
	}

	slog.DebugContext(ctx, "message replay completed",
		"task_id", taskID,
		"messages_replayed", len(messages),
	)
}
