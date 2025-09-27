package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/analytics"
	"github.com/furisto/construct/backend/api/conv"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/agent"
	"github.com/google/uuid"
)

var _ v1connect.AgentServiceHandler = (*AgentHandler)(nil)

func NewAgentHandler(db *memory.Client, analytics analytics.Client) *AgentHandler {
	return &AgentHandler{
		db:        db,
		analytics: analytics,
	}
}

type AgentHandler struct {
	db        *memory.Client
	analytics analytics.Client
	v1connect.UnimplementedAgentServiceHandler
}

func (h *AgentHandler) CreateAgent(ctx context.Context, req *connect.Request[v1.CreateAgentRequest]) (*connect.Response[v1.CreateAgentResponse], error) {
	modelID, err := uuid.Parse(req.Msg.ModelId)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid model ID format: %w", err)))
	}

	type agentModel struct {
		agent *memory.Agent
		model *memory.Model
	}

	am, err := memory.Transaction(ctx, h.db, func(tx *memory.Client) (*agentModel, error) {
		create := tx.Agent.Create().
			SetName(req.Msg.Name).
			SetInstructions(req.Msg.Instructions)

		model, err := tx.Model.Get(ctx, modelID)
		if err != nil {
			return nil, err
		}

		if !model.Enabled {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("default model is disabled"))
		}
		create.SetModel(model)

		if req.Msg.Description != "" {
			create = create.SetDescription(req.Msg.Description)
		}

		agent, err := create.Save(ctx)
		if err != nil {
			return nil, err
		}

		return &agentModel{
			agent: agent,
			model: model,
		}, nil
	})

	if err != nil {
		return nil, apiError(err)
	}

	protoAgent, err := conv.ConvertAgentToProto(am.agent)
	if err != nil {
		return nil, apiError(err)
	}

	analytics.EmitAgentCreated(h.analytics, am.agent.ID.String(), am.agent.Name, am.model.Name)

	return connect.NewResponse(&v1.CreateAgentResponse{
		Agent: protoAgent,
	}), nil
}

func (h *AgentHandler) GetAgent(ctx context.Context, req *connect.Request[v1.GetAgentRequest]) (*connect.Response[v1.GetAgentResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}

	agent, err := h.db.Agent.Query().
		Where(agent.ID(id)).
		WithModel().
		First(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoAgent, err := conv.ConvertAgentToProto(agent)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.GetAgentResponse{
		Agent: protoAgent,
	}), nil
}

func (h *AgentHandler) ListAgents(ctx context.Context, req *connect.Request[v1.ListAgentsRequest]) (*connect.Response[v1.ListAgentsResponse], error) {
	query := h.db.Agent.Query().WithModel()

	if req.Msg.Filter != nil && len(req.Msg.Filter.Names) > 0 {
		query = query.Where(agent.NameIn(req.Msg.Filter.Names...))
	}

	agents, err := query.All(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoAgents := make([]*v1.Agent, 0, len(agents))
	for _, a := range agents {
		protoAgent, err := conv.ConvertAgentToProto(a)
		if err != nil {
			return nil, apiError(err)
		}
		protoAgents = append(protoAgents, protoAgent)
	}

	return connect.NewResponse(&v1.ListAgentsResponse{
		Agents: protoAgents,
	}), nil
}

func (h *AgentHandler) UpdateAgent(ctx context.Context, req *connect.Request[v1.UpdateAgentRequest]) (*connect.Response[v1.UpdateAgentResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err)))
	}

	update := h.db.Agent.UpdateOneID(id)

	var updatedFields []string
	if req.Msg.Name != nil {
		update = update.SetName(*req.Msg.Name)
		updatedFields = append(updatedFields, "name")
	}

	if req.Msg.Description != nil {
		update = update.SetDescription(*req.Msg.Description)
		updatedFields = append(updatedFields, "description")
	}

	if req.Msg.Instructions != nil {
		update = update.SetInstructions(*req.Msg.Instructions)
		updatedFields = append(updatedFields, "instructions")
	}

	if req.Msg.ModelId != nil {
		modelID, err := uuid.Parse(*req.Msg.ModelId)
		if err != nil {
			return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid model ID format: %w", err)))
		}
		_, err = h.db.Model.Get(ctx, modelID)
		if err != nil {
			return nil, apiError(err)
		}
		update = update.SetModelID(modelID)
		updatedFields = append(updatedFields, "model_id")
	}

	updatedAgent, err := update.Save(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	updatedAgent, err = h.db.Agent.Query().
		Where(agent.ID(updatedAgent.ID)).
		WithModel().
		First(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoAgent, err := conv.ConvertAgentToProto(updatedAgent)
	if err != nil {
		return nil, apiError(err)
	}

	analytics.EmitAgentUpdated(h.analytics, updatedAgent.ID.String(), updatedAgent.Name, updatedFields)

	return connect.NewResponse(&v1.UpdateAgentResponse{
		Agent: protoAgent,
	}), nil
}

func (h *AgentHandler) DeleteAgent(ctx context.Context, req *connect.Request[v1.DeleteAgentRequest]) (*connect.Response[v1.DeleteAgentResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid agent ID format: %w", err)))
	}

	agent, err := h.db.Agent.Get(ctx, id)
	if err != nil {
		return nil, apiError(err)
	}

	if err := h.db.Agent.DeleteOne(agent).Exec(ctx); err != nil {
		return nil, apiError(fmt.Errorf("failed to delete agent: %w", err))
	}

	analytics.EmitAgentDeleted(h.analytics, agent.ID.String(), agent.Name)

	return connect.NewResponse(&v1.DeleteAgentResponse{}), nil
}
