package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/modelprovider"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/secret"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func NewModelProviderHandler(db *memory.Client) *ModelProviderHandler {
	return &ModelProviderHandler{
		db: db,
	}
}

type ModelProviderHandler struct {
	db *memory.Client
	v1connect.UnimplementedModelProviderServiceHandler
}

func (h *ModelProviderHandler) CreateProvider(ctx context.Context, req *connect.Request[v1.CreateModelProviderRequest]) (*connect.Response[v1.CreateModelProviderResponse], error) {
	providerType, err := convertProviderTypeFromProto(req.Msg.ProviderType)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	modelProvider, err := h.db.ModelProvider.Create().
		SetName(req.Msg.Name).
		SetProviderType(providerType).
		SetURL(req.Msg.Url).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create model provider: %w", err))
	}

	secretKey := secret.ModelProviderSecret(modelProvider.ID)
	if err := secret.SetSecret(secretKey, &req.Msg.ApiKey); err != nil {
		_ = h.db.ModelProvider.DeleteOne(modelProvider).Exec(ctx)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to store API key: %w", err))
	}

	modelProvider, err = h.db.ModelProvider.UpdateOne(modelProvider).
		SetSecretRef(secretKey).
		Save(ctx)
	if err != nil {
		_ = h.db.ModelProvider.DeleteOne(modelProvider).Exec(ctx)
		_ = secret.DeleteSecret(secretKey)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update model provider: %w", err))
	}

	protoMP, err := convertModelProviderToProto(modelProvider)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.CreateModelProviderResponse{
		ModelProvider: protoMP,
	}), nil
}

func (h *ModelProviderHandler) GetProvider(ctx context.Context, req *connect.Request[v1.GetModelProviderRequest]) (*connect.Response[v1.GetModelProviderResponse], error) {
	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err))
	}

	mp, err := h.db.ModelProvider.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("model provider not found: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get model provider: %w", err))
	}

	protoMP, err := convertModelProviderToProto(mp)
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

	mps, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list model providers: %w", err))
	}

	protoMPs := make([]*v1.ModelProvider, 0, len(mps))
	for _, mp := range mps {
		protoMP, err := convertModelProviderToProto(mp)
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

	mp, err := h.db.ModelProvider.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("model provider not found: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get model provider: %w", err))
	}

	update := h.db.ModelProvider.UpdateOne(mp)

	if req.Msg.Name != nil {
		update = update.SetName(*req.Msg.Name)
	}

	if req.Msg.Enabled != nil {
		update = update.SetEnabled(*req.Msg.Enabled)
	}

	if req.Msg.ApiKey != nil {
		secretKey := secret.ModelProviderSecret(mp.ID)
		if err := secret.SetSecret(secretKey, req.Msg.ApiKey); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update API key: %w", err))
		}
	}

	mp, err = update.Save(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update model provider: %w", err))
	}

	protoMP, err := convertModelProviderToProto(mp)
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

	mp, err := h.db.ModelProvider.Get(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("model provider not found: %w", err))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get model provider: %w", err))
	}

	secretKey := secret.ModelProviderSecret(mp.ID)
	if err := secret.DeleteSecret(secretKey); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete API key: %w", err))
	}

	if err := h.db.ModelProvider.DeleteOne(mp).Exec(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete model provider: %w", err))
	}

	return connect.NewResponse(&v1.DeleteModelProviderResponse{}), nil
}

func convertProviderTypeFromProto(protoType v1.ModelProviderType) (types.ModelProviderType, error) {
	switch protoType {
	case v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC:
		return types.ModelProviderTypeAnthropic, nil
	case v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI:
		return types.ModelProviderTypeOpenAI, nil
	default:
		return "", fmt.Errorf("unsupported provider type: %v", protoType)
	}
}

func convertProviderTypeToProto(dbType types.ModelProviderType) (v1.ModelProviderType, error) {
	switch dbType {
	case types.ModelProviderTypeAnthropic:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC, nil
	case types.ModelProviderTypeOpenAI:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI, nil
	default:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_UNSPECIFIED, fmt.Errorf("unsupported provider type: %v", dbType)
	}
}

func convertModelProviderToProto(mp *memory.ModelProvider) (*v1.ModelProvider, error) {
	protoType, err := convertProviderTypeToProto(mp.ProviderType)
	if err != nil {
		return nil, err
	}

	return &v1.ModelProvider{
		Id:           mp.ID.String(),
		Name:         mp.Name,
		ProviderType: protoType,
		Enabled:      mp.Enabled,
		CreatedAt:    timestamppb.New(mp.CreateTime),
		UpdatedAt:    timestamppb.New(mp.UpdateTime),
	}, nil
}

func isNotFound(err error) bool {
	return err != nil && err.Error() == "not found"
}
