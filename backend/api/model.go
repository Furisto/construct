package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/api/conv"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/model"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/google/uuid"
)

type ModelHandler struct {
	db *memory.Client
	v1connect.UnimplementedModelServiceHandler
}

func NewModelHandler(db *memory.Client) *ModelHandler {
	return &ModelHandler{
		db: db,
	}
}

var _ v1connect.ModelServiceHandler = (*ModelHandler)(nil)

func (h *ModelHandler) CreateModel(ctx context.Context, req *connect.Request[v1.CreateModelRequest]) (*connect.Response[v1.CreateModelResponse], error) {
	modelProviderID, err := uuid.Parse(req.Msg.ModelProviderId)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid model provider ID format: %w", err)))
	}

	modelProvider, err := h.db.ModelProvider.Get(ctx, modelProviderID)
	if err != nil {
		return nil, apiError(err)
	}

	capabilities := make([]types.ModelCapability, 0, len(req.Msg.Capabilities))
	for _, cap := range req.Msg.Capabilities {
		cap, err := conv.ProtoModelCapabilityToMemory(cap)
		if err != nil {
			return nil, apiError(err)
		}
		capabilities = append(capabilities, cap)
	}

	modelCreate := h.db.Model.Create().
		SetName(req.Msg.Name).
		SetModelProvider(modelProvider).
		SetContextWindow(req.Msg.ContextWindow).
		SetEnabled(true)

	if len(capabilities) > 0 {
		modelCreate.SetCapabilities(capabilities)
	}

	if req.Msg.Pricing != nil {
		inputCost, outputCost, cacheWriteCost, cacheReadCost, err := conv.ProtoModelPricingToMemory(req.Msg.Pricing)
		if err != nil {
			return nil, apiError(err)
		}
		modelCreate.
			SetInputCost(inputCost).
			SetOutputCost(outputCost).
			SetCacheWriteCost(cacheWriteCost).
			SetCacheReadCost(cacheReadCost)
	}

	model, err := modelCreate.Save(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoModel, err := conv.MemoryModelToProto(model)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.CreateModelResponse{
		Model: protoModel,
	}), nil
}

func (h *ModelHandler) GetModel(ctx context.Context, req *connect.Request[v1.GetModelRequest]) (*connect.Response[v1.GetModelResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}

	model, err := h.db.Model.Get(ctx, id)
	if err != nil {
		return nil, apiError(err)
	}

	protoModel, err := conv.MemoryModelToProto(model)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.GetModelResponse{
		Model: protoModel,
	}), nil
}

func (h *ModelHandler) ListModels(ctx context.Context, req *connect.Request[v1.ListModelsRequest]) (*connect.Response[v1.ListModelsResponse], error) {
	query := h.db.Model.Query()

	if req.Msg.Filter != nil {
		if req.Msg.Filter.ModelProviderId != nil {
			modelProviderID, err := uuid.Parse(*req.Msg.Filter.ModelProviderId)
			if err != nil {
				return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid model provider ID format: %w", err)))
			}
			query = query.Where(model.ModelProviderIDEQ(modelProviderID))
		}

		if req.Msg.Filter.Enabled != nil {
			query = query.Where(model.EnabledEQ(*req.Msg.Filter.Enabled))
		}
	}

	models, err := query.All(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoModels := make([]*v1.Model, 0, len(models))
	for _, m := range models {
		protoModel, err := conv.MemoryModelToProto(m)
		if err != nil {
			return nil, apiError(err)
		}
		protoModels = append(protoModels, protoModel)
	}

	return connect.NewResponse(&v1.ListModelsResponse{
		Models: protoModels,
	}), nil
}

func (h *ModelHandler) UpdateModel(ctx context.Context, req *connect.Request[v1.UpdateModelRequest]) (*connect.Response[v1.UpdateModelResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}

	_, err = h.db.Model.Get(ctx, id)
	if err != nil {
		return nil, apiError(err)
	}

	update := h.db.Model.UpdateOneID(id)

	if req.Msg.Name != nil {
		update = update.SetName(*req.Msg.Name)
	}

	if req.Msg.ModelProviderId != nil {
		modelProviderID, err := uuid.Parse(*req.Msg.ModelProviderId)
		if err != nil {
			return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid model provider ID format: %w", err)))
		}

		_, err = h.db.ModelProvider.Get(ctx, modelProviderID)
		if err != nil {
			return nil, apiError(err)
		}

		update = update.SetModelProviderID(modelProviderID)
	}

	if len(req.Msg.Capabilities) > 0 {
		capabilities := make([]types.ModelCapability, 0, len(req.Msg.Capabilities))
		for _, cap := range req.Msg.Capabilities {
			switch cap {
			case v1.ModelCapability_MODEL_CAPABILITY_IMAGE:
				capabilities = append(capabilities, types.ModelCapabilityImage)
			case v1.ModelCapability_MODEL_CAPABILITY_COMPUTER_USE:
				capabilities = append(capabilities, types.ModelCapabilityComputerUse)
			case v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE:
				capabilities = append(capabilities, types.ModelCapabilityPromptCache)
			case v1.ModelCapability_MODEL_CAPABILITY_THINKING:
				capabilities = append(capabilities, types.ModelCapabilityExtendedThinking)
			}
		}
		update = update.SetCapabilities(capabilities)
	}

	if req.Msg.Pricing != nil {
		inputCost, outputCost, cacheWriteCost, cacheReadCost, err := conv.ProtoModelPricingToMemory(req.Msg.Pricing)
		if err != nil {
			return nil, apiError(err)
		}
		update = update.
			SetInputCost(inputCost).
			SetOutputCost(outputCost).
			SetCacheWriteCost(cacheWriteCost).
			SetCacheReadCost(cacheReadCost)
	}

	if req.Msg.ContextWindow != nil {
		update = update.SetContextWindow(*req.Msg.ContextWindow)
	}

	if req.Msg.Enabled != nil {
		update = update.SetEnabled(*req.Msg.Enabled)
	}

	model, err := update.Save(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	protoModel, err := conv.MemoryModelToProto(model)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.UpdateModelResponse{
		Model: protoModel,
	}), nil
}

func (h *ModelHandler) DeleteModel(ctx context.Context, req *connect.Request[v1.DeleteModelRequest]) (*connect.Response[v1.DeleteModelResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}

	err = h.db.Model.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.DeleteModelResponse{}), nil
}
