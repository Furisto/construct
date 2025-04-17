package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/api/conv"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/agent"
	"github.com/furisto/construct/backend/memory/task"
	"github.com/google/uuid"
)

var _ v1connect.TaskServiceHandler = (*TaskHandler)(nil)

func NewTaskHandler(db *memory.Client) *TaskHandler {
	return &TaskHandler{
		db: db,
	}
}

type TaskHandler struct {
	db *memory.Client
	v1connect.UnimplementedTaskServiceHandler
}

func (h *TaskHandler) CreateTask(ctx context.Context, req *connect.Request[v1.CreateTaskRequest]) (*connect.Response[v1.CreateTaskResponse], error) {
	agentID, err := uuid.Parse(req.Msg.AgentId)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err)))
	}
	
	createdTask, err := memory.Transaction(ctx, h.db, func(tx *memory.Client) (*memory.Task, error) {
		return tx.Task.Create().
			SetAgentID(agentID).
			Save(ctx)
	})

	if err != nil {
		return nil, apiError(err)
	}

	protoTask, err := conv.ConvertTaskToProto(createdTask)
	if err != nil {
		return nil, apiError(err)
	}

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

	t, err := h.db.Task.Query().Where(task.ID(id)).WithAgent().First(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	update := t.Update()

	if req.Msg.AgentId != nil {
		agentID, err := uuid.Parse(*req.Msg.AgentId)
		if err != nil {
			return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err)))
		}

		_, err = h.db.Agent.Query().Where(agent.ID(agentID)).First(ctx)
		if err != nil {
			return nil, apiError(err)
		}

		update = update.SetAgentID(agentID)
	}

	_, err = update.Save(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	updatedTask, err := h.db.Task.Query().Where(task.ID(id)).WithAgent().First(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoTask, err := conv.ConvertTaskToProto(updatedTask)
	if err != nil {
		return nil, apiError(err)
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
