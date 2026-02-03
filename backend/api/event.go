package api

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/api/conv"
	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	memory_message "github.com/furisto/construct/backend/memory/message"
	"github.com/google/uuid"
)

var _ v1connect.EventServiceHandler = (*EventHandler)(nil)

// EventHandler implements the EventService for streaming events to clients.
type EventHandler struct {
	db          *memory.Client
	eventRouter *event.EventRouter
	v1connect.UnimplementedEventServiceHandler
}

// NewEventHandler creates a new EventHandler.
func NewEventHandler(db *memory.Client, eventRouter *event.EventRouter) *EventHandler {
	return &EventHandler{
		db:          db,
		eventRouter: eventRouter,
	}
}

// Subscribe streams events matching the filter criteria.
func (h *EventHandler) Subscribe(
	ctx context.Context,
	req *connect.Request[v1.EventSubscribeRequest],
	stream *connect.ServerStream[v1.EventSubscribeResponse],
) error {
	opts := event.SubscribeOptions{
		EventTypes:           req.Msg.EventTypes,
		TaskID:               ptrToString(req.Msg.TaskId),
		ReplayAfterMessageID: ptrToString(req.Msg.ReplayAfterMessageId),
	}

	slog.DebugContext(ctx, "event subscription started",
		"event_types", opts.EventTypes,
		"task_id", opts.TaskID,
		"replay_after_message_id", opts.ReplayAfterMessageID,
	)

	// Handle replay if task ID is specified
	// If ReplayAfterMessageID is set, replay messages after that ID
	// If ReplayAfterMessageID is empty, replay all messages for the task
	if opts.TaskID != "" {
		if err := h.replayMessages(ctx, stream, opts.TaskID, opts.ReplayAfterMessageID); err != nil {
			slog.ErrorContext(ctx, "failed to replay messages", "error", err)
			// Continue with live subscription even if replay fails
		}
	}

	// Subscribe to live events
	eventCh, cancel := h.eventRouter.Subscribe(ctx, opts)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "event subscription ended due to context cancellation")
			return nil
		case domainEvent, ok := <-eventCh:
			if !ok {
				slog.DebugContext(ctx, "event subscription ended due to channel close")
				return nil
			}

			protoEvent, err := conv.ConvertStreamEventToProto(domainEvent)
			if err != nil {
				slog.ErrorContext(ctx, "failed to convert event to proto",
					"event_type", domainEvent.Type,
					"error", err,
				)
				continue // Skip this event but continue streaming
			}

			if err := stream.Send(&v1.EventSubscribeResponse{Event: protoEvent}); err != nil {
				slog.ErrorContext(ctx, "failed to send event", "error", err)
				return err
			}
		}
	}
}

// replayMessages replays message.created events for a task.
// If afterMessageIDStr is empty, replays all messages for the task.
// If afterMessageIDStr is set, replays only messages created after that message.
func (h *EventHandler) replayMessages(
	ctx context.Context,
	stream *connect.ServerStream[v1.EventSubscribeResponse],
	taskIDStr, afterMessageIDStr string,
) error {
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		return err
	}

	query := h.db.Message.Query().
		Where(memory_message.TaskIDEQ(taskID)).
		Order(memory_message.ByCreateTime())

	// If afterMessageIDStr is specified, only replay messages after that one
	if afterMessageIDStr != "" {
		afterMessageID, err := uuid.Parse(afterMessageIDStr)
		if err != nil {
			return err
		}

		afterMessage, err := h.db.Message.Get(ctx, afterMessageID)
		if err != nil {
			return err
		}

		query = query.Where(memory_message.CreateTimeGT(afterMessage.CreateTime))
	}

	messages, err := query.All(ctx)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "replaying messages",
		"task_id", taskIDStr,
		"after_message_id", afterMessageIDStr,
		"message_count", len(messages),
	)

	// Send each message as a message.created event
	for _, msg := range messages {
		domainEvent := event.NewMessageCreatedEvent(msg)
		protoEvent, err := conv.ConvertStreamEventToProto(domainEvent)
		if err != nil {
			slog.ErrorContext(ctx, "failed to convert replayed message to proto",
				"message_id", msg.ID,
				"error", err,
			)
			continue
		}

		if err := stream.Send(&v1.EventSubscribeResponse{Event: protoEvent}); err != nil {
			return err
		}
	}

	return nil
}

func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
