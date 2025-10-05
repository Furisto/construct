package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/analytics"
	"github.com/furisto/construct/backend/api/conv"
	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/agent"
	"github.com/furisto/construct/backend/memory/extension"
	"github.com/furisto/construct/backend/memory/message"
	"github.com/furisto/construct/backend/memory/task"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ v1connect.TaskServiceHandler = (*TaskHandler)(nil)

func NewTaskHandler(db *memory.Client, messageHub *event.MessageHub, eventBus *event.Bus, runtime AgentRuntime, analytics analytics.Client) *TaskHandler {
	return &TaskHandler{
		db:         db,
		messageHub: messageHub,
		eventBus:   eventBus,
		runtime:    runtime,
		analytics:  analytics,
	}
}

type TaskHandler struct {
	db         *memory.Client
	messageHub *event.MessageHub
	eventBus   *event.Bus
	runtime    AgentRuntime
	analytics  analytics.Client
	v1connect.UnimplementedTaskServiceHandler
}

func (h *TaskHandler) CreateTask(ctx context.Context, req *connect.Request[v1.CreateTaskRequest]) (*connect.Response[v1.CreateTaskResponse], error) {
	agentID, err := uuid.Parse(req.Msg.AgentId)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err)))
	}

	createdTask, err := memory.Transaction(ctx, h.db, func(tx *memory.Client) (*memory.Task, error) {
		_, err := tx.Agent.Get(ctx, agentID)
		if err != nil {
			return nil, err
		}

		return tx.Task.Create().
			SetAgentID(agentID).
			SetProjectDirectory(req.Msg.ProjectDirectory).
			Save(ctx)
	})

	if err != nil {
		return nil, apiError(err)
	}

	protoTask, err := conv.ConvertTaskToProto(createdTask)
	if err != nil {
		return nil, apiError(err)
	}

	analytics.EmitTaskCreated(h.analytics, createdTask.ID.String(), createdTask.AgentID.String())

	return connect.NewResponse(&v1.CreateTaskResponse{
		Task: protoTask,
	}), nil
}

func (h *TaskHandler) GetTask(ctx context.Context, req *connect.Request[v1.GetTaskRequest]) (*connect.Response[v1.GetTaskResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err)))
	}

	task, err := h.db.Task.Query().Where(task.ID(id)).WithAgent().First(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoTask, err := conv.ConvertTaskToProto(task)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.GetTaskResponse{
		Task: protoTask,
	}), nil
}

func (h *TaskHandler) ListTasks(ctx context.Context, req *connect.Request[v1.ListTasksRequest]) (*connect.Response[v1.ListTasksResponse], error) {
	query := h.db.Task.Query()

	if req.Msg.Filter != nil && req.Msg.Filter.AgentId != nil {
		agentID, err := uuid.Parse(*req.Msg.Filter.AgentId)
		if err != nil {
			return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err)))
		}
		query = query.Where(task.HasAgentWith(agent.ID(agentID)))
	}

	if req.Msg.Filter != nil && req.Msg.Filter.TaskIdPrefix != nil {
		query = query.Where(extension.UUIDHasPrefix(task.FieldID, *req.Msg.Filter.TaskIdPrefix))
	}

	query.Modify(func(s *sql.Selector) {
		m := sql.Table(message.Table)
		countExpr := sql.Count("*")
		s.LeftJoin(m).On(s.C(task.FieldID), m.C(message.FieldTaskID))
		s.AppendSelect(sql.As(countExpr, "messages_count"))
		s.GroupBy(s.C(task.FieldID))

		if req.Msg.Filter != nil && req.Msg.Filter.HasMessages != nil {
			if *req.Msg.Filter.HasMessages {
				s.Having(sql.GT(countExpr, 0))
			} else {
				s.Having(sql.EQ(countExpr, 0))
			}
		}
	})

	sortField := v1.SortField_SORT_FIELD_CREATED_AT
	if req.Msg.SortField != nil {
		sortField = *req.Msg.SortField
	}

	sortOrder := v1.SortOrder_SORT_ORDER_DESC
	if req.Msg.SortOrder != nil {
		sortOrder = *req.Msg.SortOrder
	}

	switch sortField {
	case v1.SortField_SORT_FIELD_CREATED_AT:
		if sortOrder == v1.SortOrder_SORT_ORDER_ASC {
			query = query.Order(task.ByCreateTime(sql.OrderAsc()))
		} else {
			query = query.Order(task.ByCreateTime(sql.OrderDesc()))
		}
	case v1.SortField_SORT_FIELD_UPDATED_AT:
		if sortOrder == v1.SortOrder_SORT_ORDER_ASC {
			query = query.Order(task.ByUpdateTime(sql.OrderAsc()))
		} else {
			query = query.Order(task.ByUpdateTime(sql.OrderDesc()))
		}
	}

	if req.Msg.PageSize != nil {
		query = query.Limit(int(*req.Msg.PageSize))
	}

	tasks, err := query.WithAgent().All(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoTasks := make([]*v1.Task, 0, len(tasks))
	for _, t := range tasks {
		protoTask, err := conv.ConvertTaskToProto(t)
		if err != nil {
			return nil, apiError(err)
		}

		var mc int64
		if v, err := t.Value("messages_count"); err == nil {
			if n, ok := v.(int64); ok {
				mc = n
			}
		}
		protoTask.Status.MessageCount = mc
		protoTasks = append(protoTasks, protoTask)
	}

	return connect.NewResponse(&v1.ListTasksResponse{
		Tasks: protoTasks,
	}), nil
}

func (h *TaskHandler) UpdateTask(ctx context.Context, req *connect.Request[v1.UpdateTaskRequest]) (*connect.Response[v1.UpdateTaskResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err)))
	}

	var updatedFields []string
	updatedTask, err := memory.Transaction(ctx, h.db, func(tx *memory.Client) (*memory.Task, error) {
		t, err := tx.Task.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		update := t.Update()

		if req.Msg.AgentId != nil {
			agentID, err := uuid.Parse(*req.Msg.AgentId)
			if err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err))
			}

			_, err = tx.Agent.Get(ctx, agentID)
			if err != nil {
				return nil, err
			}

			update = update.SetAgentID(agentID)
			updatedFields = append(updatedFields, "agent_id")
		}

		return update.Save(ctx)
	})

	if err != nil {
		return nil, apiError(err)
	}

	protoTask, err := conv.ConvertTaskToProto(updatedTask)
	if err != nil {
		return nil, apiError(err)
	}

	analytics.EmitTaskUpdated(h.analytics, updatedTask.ID.String(), updatedFields)

	for _, field := range updatedFields {
		if field == "agent_id" {
			taskEvent := &v1.TaskEvent{
				TaskId:    updatedTask.ID.String(),
				Timestamp: timestamppb.Now(),
			}
			h.messageHub.Publish(updatedTask.ID, &v1.SubscribeResponse{
				Event: &v1.SubscribeResponse_TaskEvent{
					TaskEvent: taskEvent,
				},
			})
			break
		}
	}

	return connect.NewResponse(&v1.UpdateTaskResponse{
		Task: protoTask,
	}), nil
}

func (h *TaskHandler) DeleteTask(ctx context.Context, req *connect.Request[v1.DeleteTaskRequest]) (*connect.Response[v1.DeleteTaskResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err)))
	}

	if err := h.db.Task.DeleteOneID(id).Exec(ctx); err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.DeleteTaskResponse{}), nil
}

func (h *TaskHandler) Subscribe(ctx context.Context, req *connect.Request[v1.SubscribeRequest], stream *connect.ServerStream[v1.SubscribeResponse]) error {
	taskID, err := uuid.Parse(req.Msg.TaskId)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err))
	}

	_, err = h.db.Task.Get(ctx, taskID)
	if err != nil {
		return apiError(err)
	}

	event.Publish(h.eventBus, event.TaskEvent{
		TaskID: taskID,
	})

	for response, err := range h.messageHub.Subscribe(ctx, taskID) {
		if err != nil {
			return apiError(err)
		}

		if err := stream.Send(response); err != nil {
			return err
		}
	}
	return nil
}

func (h *TaskHandler) SuspendTask(ctx context.Context, req *connect.Request[v1.SuspendTaskRequest]) (*connect.Response[v1.SuspendTaskResponse], error) {
	taskID, err := uuid.Parse(req.Msg.TaskId)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err)))
	}

	h.runtime.CancelTask(taskID)
	return connect.NewResponse(&v1.SuspendTaskResponse{}), nil
}
