package api

import (
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/api/conv"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/modelprovider"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/secret"
	"github.com/google/uuid"
)

var _ v1connect.ModelProviderServiceHandler = (*ModelProviderHandler)(nil)

func NewModelProviderHandler(db *memory.Client, encryption *secret.Client) *ModelProviderHandler {
	return &ModelProviderHandler{
		db:         db,
		encryption: encryption,
	}
}

type ModelProviderHandler struct {
	db         *memory.Client
	encryption *secret.Client
	v1connect.UnimplementedModelProviderServiceHandler
}

func (h *ModelProviderHandler) CreateModelProvider(ctx context.Context, req *connect.Request[v1.CreateModelProviderRequest]) (*connect.Response[v1.CreateModelProviderResponse], error) {
	providerType, err := conv.ConvertProviderTypeFromProto(req.Msg.ProviderType)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, err))
	}

	jsonSecret, err := marshalAuthToJson(req.Msg.Authentication)
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to marshal authentication config: %w", err))
	}

	modelProviderID := uuid.New()
	encryptedSecret, err := h.encryption.Encrypt(jsonSecret, []byte(secret.ModelProviderSecret(modelProviderID)))
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to encrypt API key"))
	}

	modelProvider, err := memory.Transaction(ctx, h.db, func(tx *memory.Client) (*memory.ModelProvider, error) {
		modelProvider, err := tx.ModelProvider.Create().
			SetID(modelProviderID).
			SetName(req.Msg.Name).
			SetProviderType(providerType).
			SetEnabled(true).
			SetSecret(encryptedSecret).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to insert model provider: %w", err)
		}

		converter := conv.NewModelConverter()
		supportedModels := model.SupportedModels(model.Provider(providerType))
		models := make([]*memory.ModelCreate, 0, len(supportedModels))
		for _, m := range supportedModels {
			capabilities, err := converter.ConvertModelCapabilitiesToMemory(m.Capabilities)
			if err != nil {
				return nil, err
			}
			models = append(models, h.db.Model.Create().
				SetModelProvider(modelProvider).
				SetName(m.Name).
				SetContextWindow(m.ContextWindow).
				SetCapabilities(capabilities).
				SetInputCost(m.Pricing.Input).
				SetOutputCost(m.Pricing.Output).
				SetCacheWriteCost(m.Pricing.CacheWrite).
				SetCacheReadCost(m.Pricing.CacheRead).
				SetEnabled(true))
		}

		_, err = tx.Model.CreateBulk(models...).Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to insert models: %w", err)
		}

		return modelProvider, nil
	})

	if err != nil {
		return nil, apiError(err)
	}

	converter := conv.NewModelProviderConverter()
	apiModelProvider, err := converter.ConvertIntoProto(modelProvider)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.CreateModelProviderResponse{
		ModelProvider: apiModelProvider,
	}), nil
}

func (h *ModelProviderHandler) GetModelProvider(ctx context.Context, req *connect.Request[v1.GetModelProviderRequest]) (*connect.Response[v1.GetModelProviderResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}

	modelProvider, err := h.db.ModelProvider.Get(ctx, id)
	if err != nil {
		return nil, apiError(err)
	}

	converter := conv.NewModelProviderConverter()
	apiModelProvider, err := converter.ConvertIntoProto(modelProvider)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.GetModelProviderResponse{
		ModelProvider: apiModelProvider,
	}), nil
}

func (h *ModelProviderHandler) ListModelProviders(ctx context.Context, req *connect.Request[v1.ListModelProvidersRequest]) (*connect.Response[v1.ListModelProvidersResponse], error) {
	query := h.db.ModelProvider.Query()

	if req.Msg.Filter != nil {
		if req.Msg.Filter.Enabled != nil {
			query = query.Where(modelprovider.Enabled(*req.Msg.Filter.Enabled))
		}

		if len(req.Msg.Filter.ProviderTypes) > 0 {
			providerTypes := make([]types.ModelProviderType, 0, len(req.Msg.Filter.ProviderTypes))
			for _, providerType := range req.Msg.Filter.ProviderTypes {
				providerType, err := conv.ConvertProviderTypeFromProto(providerType)
				if err != nil {
					return nil, apiError(err)
				}
				providerTypes = append(providerTypes, providerType)
			}
			query = query.Where(modelprovider.ProviderTypeIn(providerTypes...))
		}
	}

	modelProviders, err := query.All(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	converter := conv.NewModelProviderConverter()
	protoModelProviders := make([]*v1.ModelProvider, 0, len(modelProviders))
	for _, mp := range modelProviders {
		protoModelProvider, err := converter.ConvertIntoProto(mp)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		protoModelProviders = append(protoModelProviders, protoModelProvider)
	}

	return connect.NewResponse(&v1.ListModelProvidersResponse{
		ModelProviders: protoModelProviders,
	}), nil
}

func (h *ModelProviderHandler) UpdateModelProvider(ctx context.Context, req *connect.Request[v1.UpdateModelProviderRequest]) (*connect.Response[v1.UpdateModelProviderResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err))
	}

	modelProvider, err := h.db.ModelProvider.Get(ctx, id)
	if err != nil {
		return nil, apiError(err)
	}

	update := h.db.ModelProvider.UpdateOne(modelProvider)

	if req.Msg.Name != nil {
		update = update.SetName(*req.Msg.Name)
	}

	if req.Msg.Enabled != nil {
		update = update.SetEnabled(*req.Msg.Enabled)
	}

	if req.Msg.Authentication != nil {
		jsonSecret, err := marshalAuthToJson(req.Msg.Authentication)
		if err != nil {
			return nil, apiError(fmt.Errorf("failed to marshal API key: %w", err))
		}

		encryptedSecret, err := h.encryption.Encrypt(jsonSecret, []byte(secret.ModelProviderSecret(id)))
		if err != nil {
			return nil, apiError(fmt.Errorf("failed to encrypt API key: %w", err))
		}

		update = update.SetSecret(encryptedSecret)
	}

	modelProvider, err = update.Save(ctx)
	if err != nil {
		return nil, apiError(err)
	}

	converter := conv.NewModelProviderConverter()
	protoModelProvider, err := converter.ConvertIntoProto(modelProvider)
	if err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.UpdateModelProviderResponse{
		ModelProvider: protoModelProvider,
	}), nil
}

func (h *ModelProviderHandler) DeleteModelProvider(ctx context.Context, req *connect.Request[v1.DeleteModelProviderRequest]) (*connect.Response[v1.DeleteModelProviderResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}

	modelProvider, err := h.db.ModelProvider.Get(ctx, id)
	if err != nil {
		return nil, apiError(err)
	}

	if err := h.db.ModelProvider.DeleteOne(modelProvider).Exec(ctx); err != nil {
		return nil, apiError(fmt.Errorf("failed to delete model provider: %w", err))
	}

	return connect.NewResponse(&v1.DeleteModelProviderResponse{}), nil
}

func marshalAuthToJson(config any) ([]byte, error) {
	switch config := config.(type) {
	case *v1.CreateModelProviderRequest_ApiKey:
		return json.Marshal(map[string]interface{}{
			"apiKey": config.ApiKey,
		})
	default:
		return nil, fmt.Errorf("unsupported authentication config type: %T", config)
	}
}
