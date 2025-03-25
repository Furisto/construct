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
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/secret"
	"github.com/google/uuid"
)

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

func (h *ModelProviderHandler) CreateProvider(ctx context.Context, req *connect.Request[v1.CreateModelProviderRequest]) (*connect.Response[v1.CreateModelProviderResponse], error) {
	providerType, err := conv.ConvertProviderTypeFromProto(req.Msg.ProviderType)
	if err != nil {
		return nil, apiError(err)
	}

	jsonSecret, err := json.Marshal(APIKey{Key: req.Msg.ApiKey})
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to marshal API key: %w", err))
	}

	modelProviderID := uuid.New()
	encryptedSecret, err := h.encryption.Encrypt(jsonSecret, []byte(secret.ModelProviderSecret(modelProviderID)))
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to encrypt API key: %w", err))
	}

	modelProvider, err := memory.Transaction(ctx, h.db, func(tx *memory.Client) (*memory.ModelProvider, error) {
		modelProvider, err := h.db.ModelProvider.Create().
			SetID(modelProviderID).
			SetName(req.Msg.Name).
			SetProviderType(providerType).
			SetURL(req.Msg.Url).
			SetEnabled(true).
			SetSecret(encryptedSecret).
			Save(ctx)
		if err != nil {
			return nil, err
		}

		models := make([]*memory.ModelCreate, 0, len(model.SupportedModels(model.Provider(providerType))))
		for _, m := range model.SupportedModels(model.Provider(providerType)) {
			models = append(models, h.db.Model.Create().
				SetName(m.Name).
				SetContextWindow(m.ContextWindow).
				SetEnabled(true))
		}

		_, err = h.db.Model.CreateBulk(models...).Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create models: %w", err)
		}

		return modelProvider, nil
	})

	converter := conv.NewModelProviderConverter()
	apiModelProvider, err := converter.ConvertIntoProto(modelProvider)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.CreateModelProviderResponse{
		ModelProvider: apiModelProvider,
	}), nil
}

func (h *ModelProviderHandler) GetProvider(ctx context.Context, req *connect.Request[v1.GetModelProviderRequest]) (*connect.Response[v1.GetModelProviderResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err))
	}

	modelProvider, err := h.db.ModelProvider.Get(ctx, id)
	if err != nil {
		if memory.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	converter := conv.NewModelProviderConverter()
	protoMP, err := converter.ConvertIntoProto(modelProvider)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.GetModelProviderResponse{
		ModelProvider: protoMP,
	}), nil
}

func (h *ModelProviderHandler) ListProviders(ctx context.Context, req *connect.Request[v1.ListModelProvidersRequest]) (*connect.Response[v1.ListModelProvidersResponse], error) {
	query := h.db.ModelProvider.Query()

	if req.Msg.Filter != nil {
		query = query.Where(modelprovider.Enabled(req.Msg.Filter.Enabled))
	}

	modelProviders, err := query.All(ctx)
	if err != nil {
		if memory.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	converter := conv.NewModelProviderConverter()
	protoMPs := make([]*v1.ModelProvider, 0, len(modelProviders))
	for _, mp := range modelProviders {
		protoMP, err := converter.ConvertIntoProto(mp)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		protoMPs = append(protoMPs, protoMP)
	}

	return connect.NewResponse(&v1.ListModelProvidersResponse{
		ModelProviders: protoMPs,
	}), nil
}

func (h *ModelProviderHandler) UpdateProvider(ctx context.Context, req *connect.Request[v1.UpdateModelProviderRequest]) (*connect.Response[v1.UpdateModelProviderResponse], error) {
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

	if req.Msg.ApiKey != nil {
		secretKey := secret.ModelProviderSecret(modelProvider.ID)
		if err := secret.SetSecret(secretKey, req.Msg.ApiKey); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update API key: %w", err))
		}
	}

	modelProvider, err = update.Save(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update model provider: %w", err))
	}

	converter := conv.NewModelProviderConverter()
	protoMP, err := converter.ConvertIntoProto(modelProvider)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.UpdateModelProviderResponse{
		ModelProvider: protoMP,
	}), nil
}

func (h *ModelProviderHandler) DeleteProvider(ctx context.Context, req *connect.Request[v1.DeleteModelProviderRequest]) (*connect.Response[v1.DeleteModelProviderResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err))
	}

	modelProvider, err := h.db.ModelProvider.Get(ctx, id)
	if err != nil {
		return nil, apiError(err)
	}

	secretKey := secret.ModelProviderSecret(modelProvider.ID)
	if err := secret.DeleteSecret(secretKey); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete API key: %w", err))
	}

	if err := h.db.ModelProvider.DeleteOne(modelProvider).Exec(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete model provider: %w", err))
	}

	return connect.NewResponse(&v1.DeleteModelProviderResponse{}), nil
}

type APIKey struct {
	Key string `json:"key"`
}
