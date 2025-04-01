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
	"github.com/google/uuid"
)

var _ v1connect.MessageServiceHandler = (*MessageHandler)(nil)

func NewMessageHandler(db *memory.Client) *MessageHandler {
	return &MessageHandler{
		db: db,
	}
}

type MessageHandler struct {
	db *memory.Client
	v1connect.UnimplementedMessageServiceHandler
}

func (h *MessageHandler) CreateMessage(ctx context.Context, req *connect.Request[v1.CreateMessageRequest]) (*connect.Response[v1.CreateMessageResponse], error) {
	taskID, err := uuid.Parse(req.Msg.TaskId)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err)))
	}

	task, err := h.db.Task.Get(ctx, taskID)
	if err != nil {
		return nil, apiError(err)
	}

	content := &types.MessageContent{
		Blocks: []types.MessageContentBlock{
			{
				Type: types.MessageContentBlockTypeText,
				Text: req.Msg.Content,
			},
		},
	}

	msg, err := h.db.Message.Create().
		SetTask(task).
		SetContent(content).
		SetRole(types.MessageRoleUser).
		Save(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoMsg, err := conv.ConvertMessageToProto(msg)
	if err != nil {
		return nil, apiError(err)
	}

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

	protoMsg, err := conv.ConvertMessageToProto(msg)
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
			var role types.MessageRole
			switch *req.Msg.Filter.Role {
			case v1.MessageRole_MESSAGE_ROLE_USER:
				role = types.MessageRoleUser
			case v1.MessageRole_MESSAGE_ROLE_ASSISTANT:
				role = types.MessageRoleAssistant
			}
			query = query.Where(message.RoleEQ(role))
		}
	}

	messages, err := query.All(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoMessages := make([]*v1.Message, 0, len(messages))
	for _, m := range messages {
		protoMsg, err := conv.ConvertMessageToProto(m)
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
		Blocks: []types.MessageContentBlock{
			{
				Type: types.MessageContentBlockTypeText,
				Text: req.Msg.Content,
			},
		},
	}

	msg, err := h.db.Message.UpdateOneID(id).
		SetContent(content).
		Save(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoMsg, err := conv.ConvertMessageToProto(msg)
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
	return nil

}
