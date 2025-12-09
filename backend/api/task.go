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
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/memory/task"
	"github.com/google/uuid"
)

var _ v1connect.TaskServiceHandler = (*TaskHandler)(nil)

func NewTaskHandler(db *memory.Client, eventBus *event.Bus, runtime AgentRuntime, analytics analytics.Client) *TaskHandler {
	return &TaskHandler{
		memory:    db,
		eventBus:  eventBus,
		runtime:   runtime,
		analytics: analytics,
	}
}

type TaskHandler struct {
	memory    *memory.Client
	eventBus  *event.Bus
	runtime   AgentRuntime
	analytics analytics.Client
	v1connect.UnimplementedTaskServiceHandler
}

func (h *TaskHandler) CreateTask(ctx context.Context, req *connect.Request[v1.CreateTaskRequest]) (*connect.Response[v1.CreateTaskResponse], error) {
	agentID, err := uuid.Parse(req.Msg.AgentId)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err)))
	}

	createdTask, err := memory.Transaction(ctx, h.memory, func(tx *memory.Client) (*memory.Task, error) {
		_, err := tx.Agent.Get(ctx, agentID)
		if err != nil {
			return nil, err
		}

		taskCreate := tx.Task.Create().
			SetAgentID(agentID).
			SetProjectDirectory(req.Msg.ProjectDirectory)

		if req.Msg.Description != "" {
			taskCreate = taskCreate.SetDescription(req.Msg.Description)
		}

		return taskCreate.Save(ctx)
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

	task, err := h.memory.Task.Query().Where(task.ID(id)).WithAgent().First(ctx)
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
	query := h.memory.Task.Query()

	if req.Msg.Filter != nil && req.Msg.Filter.AgentId != nil {
		agentID, err := uuid.Parse(*req.Msg.Filter.AgentId)
		if err != nil {
			return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err)))
		}
		query = query.Where(task.HasAgentWith(agent.ID(agentID)))
	}

	if req.Msg.Filter != nil && req.Msg.Filter.TaskIdPrefix != nil {
		query = query.Where(extension.UUIDHasPrefix(task.Table, task.FieldID, *req.Msg.Filter.TaskIdPrefix))
	}

	query.Modify(func(s *sql.Selector) {
		m := sql.Table(message.Table).As("t1")
		countExpr := sql.Count(m.C(message.FieldTaskID))
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
	updatedTask, err := memory.Transaction(ctx, h.memory, func(tx *memory.Client) (*memory.Task, error) {
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
			event.Publish(h.eventBus, event.TaskEvent{
				TaskID: updatedTask.ID,
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

	if err := h.memory.Task.DeleteOneID(id).Exec(ctx); err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.DeleteTaskResponse{}), nil
}

func (h *TaskHandler) Subscribe(ctx context.Context, req *connect.Request[v1.SubscribeRequest], stream *connect.ServerStream[v1.SubscribeResponse]) error {
	taskID, err := uuid.Parse(req.Msg.TaskId)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err))
	}

	_, err = h.memory.Task.Get(ctx, taskID)
	if err != nil {
		return apiError(err)
	}

	event.Publish(h.eventBus, event.TaskEvent{
		TaskID: taskID,
	})

	err = h.publishTaskEvents(ctx, taskID, stream)
	if err != nil {
		return apiError(err)
	}

	return nil
}

func (h *TaskHandler) SuspendTask(ctx context.Context, req *connect.Request[v1.SuspendTaskRequest]) (*connect.Response[v1.SuspendTaskResponse], error) {
	taskID, err := uuid.Parse(req.Msg.TaskId)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid task ID format: %w", err)))
	}

	var childIDs []uuid.UUID
	_, err = memory.Transaction(ctx, h.memory, func(tx *memory.Client) (*memory.Task, error) {
		_, err = h.memory.Task.UpdateOneID(taskID).SetPhase(types.TaskPhaseSuspended).Save(ctx)
		if err != nil {
			return nil, err
		}

		children, err := tx.Task.Query().Where(task.ParentTaskIDEQ(taskID)).All(ctx)
		if err != nil {
			return nil, err
		}

		for _, child := range children {
			childIDs = append(childIDs, child.ID)
			_, err = child.Update().SetDesiredPhase(types.TaskPhaseSuspended).Save(ctx)
			if err != nil {
				return nil, err
			}
		}

		return nil, nil
	})

	if err != nil {
		return nil, apiError(err)
	}

	event.Publish(h.eventBus, event.TaskSuspendedEvent{
		TaskID: taskID,
	})

	for _, childID := range childIDs {
		event.Publish(h.eventBus, event.TaskSuspendedEvent{
			TaskID: childID,
		})
	}

	return connect.NewResponse(&v1.SuspendTaskResponse{}), nil
}

func (h *TaskHandler) publishTaskEvents(ctx context.Context, taskID uuid.UUID, stream *connect.ServerStream[v1.SubscribeResponse]) error {
	messages, err := h.memory.Message.Query().Where(message.TaskIDEQ(taskID), message.ProcessedTimeNotNil()).Order(message.ByProcessedTime(), memory.Asc()).All(ctx)
	if err != nil {
		return err
	}

	msgCh, msgSub := event.SubscribeChannel(h.eventBus, 10, func(event event.MessageEvent) bool {
		return event.TaskID == taskID
	})
	defer msgSub.Unsubscribe()

	taskCh, taskSub := event.SubscribeChannel(h.eventBus, 10, func(event event.TaskEvent) bool {
		return event.TaskID == taskID
	})
	defer taskSub.Unsubscribe()

	for _, m := range messages {
		protoMessage, err := conv.ConvertMemoryMessageToProto(m)
		if err != nil {
			return err
		}
		stream.Send(&v1.SubscribeResponse{
			Event: &v1.SubscribeResponse_Message{
				Message: protoMessage,
			},
		})
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case message := <-msgCh:
			m, err := h.memory.Message.Get(ctx, message.MessageID)
			if err != nil {
				return err
			}
			protoMessage, err := conv.ConvertMemoryMessageToProto(m)
			if err != nil {
				return err
			}
			stream.Send(&v1.SubscribeResponse{
				Event: &v1.SubscribeResponse_Message{
					Message: protoMessage,
				},
			})
		case task := <-taskCh:
			stream.Send(&v1.SubscribeResponse{
				Event: &v1.SubscribeResponse_TaskEvent{
					TaskEvent: &v1.TaskEvent{
						TaskId: task.TaskID.String(),
					},
				},
			})
		}
	}
}
