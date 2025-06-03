package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/api/conv"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/message"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/memory/task"
	"github.com/furisto/construct/backend/stream"
	"github.com/google/uuid"
)

var _ v1connect.MessageServiceHandler = (*MessageHandler)(nil)

func NewMessageHandler(db *memory.Client, runtime AgentRuntime, messageHub *stream.EventHub) *MessageHandler {
	return &MessageHandler{
		db:         db,
		runtime:    runtime,
		messageHub: messageHub,
	}
}

type MessageHandler struct {
	db         *memory.Client
	runtime    AgentRuntime
	messageHub *stream.EventHub
	v1connect.UnimplementedMessageServiceHandler
}

func (h *MessageHandler) CreateMessage(ctx context.Context, req *connect.Request[v1.CreateMessageRequest]) (*connect.Response[v1.CreateMessageResponse], error) {
	taskID, err := uuid.Parse(req.Msg.TaskId)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err)))
	}

	msg, err := memory.Transaction(ctx, h.db, func(tx *memory.Client) (*memory.Message, error) {
		task, err := h.db.Task.Get(ctx, taskID)
		if err != nil {
			return nil, err
		}

		content := &types.MessageContent{
			Blocks: []types.MessageBlock{
				{
					Kind:    types.MessageBlockKindText,
					Payload: req.Msg.Content,
				},
			},
		}

		return h.db.Message.Create().
			SetTask(task).
			SetContent(content).
			SetSource(types.MessageSourceUser).
			Save(ctx)
	})

	if err != nil {
		return nil, apiError(err)
	}

	protoMsg, err := conv.ConvertMemoryMessageToProto(msg)
	if err != nil {
		return nil, apiError(err)
	}

	h.runtime.TriggerReconciliation(taskID)

	return connect.NewResponse(&v1.CreateMessageResponse{
		Message: protoMsg,
	}), nil
}

func (h *MessageHandler) GetMessage(ctx context.Context, req *connect.Request[v1.GetMessageRequest]) (*connect.Response[v1.GetMessageResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}
	
	msg, err := h.db.Message.Query().
		Where(message.ID(id)).
		First(ctx)

	if err != nil {
		return nil, apiError(err)
	}

	protoMsg, err := conv.ConvertMemoryMessageToProto(msg)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.GetMessageResponse{
		Message: protoMsg,
	}), nil
}

func (h *MessageHandler) ListMessages(ctx context.Context, req *connect.Request[v1.ListMessagesRequest]) (*connect.Response[v1.ListMessagesResponse], error) {
	query := h.db.Message.Query().WithTask()

	if req.Msg.Filter != nil {
		if req.Msg.Filter.TaskId != nil {
			taskID, err := uuid.Parse(*req.Msg.Filter.TaskId)
			if err != nil {
				return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err)))
			}
			query = query.Where(message.HasTaskWith(task.IDEQ(taskID)))
		}

		if req.Msg.Filter.AgentId != nil {
			agentID, err := uuid.Parse(*req.Msg.Filter.AgentId)
			if err != nil {
				return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err)))
			}
			query = query.Where(message.AgentIDEQ(agentID))
		}

		if req.Msg.Filter.Role != nil {
			var role types.MessageSource
			switch *req.Msg.Filter.Role {
			case v1.MessageRole_MESSAGE_ROLE_USER:
				role = types.MessageSourceUser
			case v1.MessageRole_MESSAGE_ROLE_ASSISTANT:
				role = types.MessageSourceAssistant
			}
			query = query.Where(message.SourceEQ(role))
		}
	}

	messages, err := query.All(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoMessages := make([]*v1.Message, 0, len(messages))
	for _, m := range messages {
		protoMsg, err := conv.ConvertMemoryMessageToProto(m)
		if err != nil {
			return nil, apiError(err)
		}
		protoMessages = append(protoMessages, protoMsg)
	}

	return connect.NewResponse(&v1.ListMessagesResponse{
		Messages: protoMessages,
	}), nil
}

func (h *MessageHandler) UpdateMessage(ctx context.Context, req *connect.Request[v1.UpdateMessageRequest]) (*connect.Response[v1.UpdateMessageResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}

	content := &types.MessageContent{
		Blocks: []types.MessageBlock{
			{
				Kind:    types.MessageBlockKindText,
				Payload: req.Msg.Content,
			},
		},
	}

	msg, err := h.db.Message.UpdateOneID(id).
		SetContent(content).
		Save(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoMsg, err := conv.ConvertMemoryMessageToProto(msg)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.UpdateMessageResponse{
		Message: protoMsg,
	}), nil
}

func (h *MessageHandler) DeleteMessage(ctx context.Context, req *connect.Request[v1.DeleteMessageRequest]) (*connect.Response[v1.DeleteMessageResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}

	err = h.db.Message.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.DeleteMessageResponse{}), nil
}

func (h *MessageHandler) Subscribe(ctx context.Context, req *connect.Request[v1.SubscribeRequest], stream *connect.ServerStream[v1.SubscribeResponse]) error {
	taskID, err := uuid.Parse(req.Msg.TaskId)
	if err != nil {
		return apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err)))
	}

	for r, err := range h.messageHub.Subscribe(ctx, taskID) {
		if err != nil {
			return apiError(err)
		}

		stream.Send(r)
	}

	return nil

}
