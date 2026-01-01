package api

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/api/auth"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/memory/token"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ v1connect.AuthServiceHandler = (*AuthHandler)(nil)

type AuthHandler struct {
	db            *memory.Client
	tokenProvider *auth.TokenProvider
	v1connect.UnimplementedAuthServiceHandler
}

func NewAuthHandler(db *memory.Client, tokenProvider *auth.TokenProvider) *AuthHandler {
	return &AuthHandler{
		db:            db,
		tokenProvider: tokenProvider,
	}
}

func (h *AuthHandler) CreateToken(ctx context.Context, req *connect.Request[v1.CreateTokenRequest]) (*connect.Response[v1.CreateTokenResponse], error) {
	identity := auth.FromContext(ctx)
	if identity == nil || !identity.IsAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("admin privileges required"))
	}

	if req.Msg.Name == "" {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required")))
	}

	expiresIn := auth.DefaultTokenExpiry
	if req.Msg.ExpiresIn != nil {
		expiresIn = req.Msg.ExpiresIn.AsDuration()
		if expiresIn > auth.MaxTokenExpiry {
			return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("expires_in exceeds maximum of %v", auth.MaxTokenExpiry)))
		}
	}

	plaintext, hash, err := h.tokenProvider.GenerateToken()
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to generate token: %w", err))
	}

	expiresAt := time.Now().Add(expiresIn)

	create := h.db.Token.Create().
		SetName(req.Msg.Name).
		SetType(types.TokenTypeAPIToken).
		SetTokenHash(hash).
		SetExpiresAt(expiresAt)

	if req.Msg.Description != nil {
		create = create.SetDescription(*req.Msg.Description)
	}

	_, err = create.Save(ctx)
	if err != nil {
		if memory.IsConstraintError(err) {
			return nil, apiError(connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("token with name %q already exists", req.Msg.Name)))
		}
		return nil, apiError(fmt.Errorf("failed to save token: %w", err))
	}

	return connect.NewResponse(&v1.CreateTokenResponse{
		Token:     plaintext,
		ExpiresAt: timestamppb.New(expiresAt),
	}), nil
}

func (h *AuthHandler) CreateSetupCode(ctx context.Context, req *connect.Request[v1.CreateSetupCodeRequest]) (*connect.Response[v1.CreateSetupCodeResponse], error) {
	identity := auth.FromContext(ctx)
	if identity == nil || !identity.IsAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("admin privileges required"))
	}

	if req.Msg.TokenName == "" {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("token_name is required")))
	}

	codeExpiry := auth.DefaultSetupExpiry
	if req.Msg.ExpiresIn != nil {
		codeExpiry = req.Msg.ExpiresIn.AsDuration()
		if codeExpiry > auth.MaxSetupExpiry {
			return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("expires_in exceeds maximum of %v", auth.MaxSetupExpiry)))
		}
	}

	tokenExpiry := auth.DefaultTokenExpiry
	if req.Msg.TokenExpiresIn != nil {
		tokenExpiry = req.Msg.TokenExpiresIn.AsDuration()
		if tokenExpiry > auth.MaxTokenExpiry {
			return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("token_expires_in exceeds maximum of %v", auth.MaxTokenExpiry)))
		}
	}

	setupCode, err := h.tokenProvider.CreateSetupCode(req.Msg.TokenName, codeExpiry, tokenExpiry)
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to create setup code: %w", err))
	}

	return connect.NewResponse(&v1.CreateSetupCodeResponse{
		SetupCode: setupCode.Code,
		ExpiresAt: timestamppb.New(setupCode.ExpiresAt),
	}), nil
}

func (h *AuthHandler) ListTokens(ctx context.Context, req *connect.Request[v1.ListTokensRequest]) (*connect.Response[v1.ListTokensResponse], error) {
	identity := auth.FromContext(ctx)
	if identity == nil || !identity.IsAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("admin privileges required"))
	}

	query := h.db.Token.Query().Where(token.TypeEQ(types.TokenTypeAPIToken))

	if req.Msg.NamePrefix != "" {
		query = query.Where(token.NameHasPrefix(req.Msg.NamePrefix))
	}

	if !req.Msg.IncludeExpired {
		query = query.Where(token.ExpiresAtGT(time.Now()))
	}

	query = query.Order(memory.Desc(token.FieldCreateTime))

	tokens, err := query.All(ctx)
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to query tokens: %w", err))
	}

	now := time.Now()
	protoTokens := make([]*v1.TokenInfo, 0, len(tokens))
	for _, tok := range tokens {
		isActive := tok.ExpiresAt.After(now)

		protoToken := &v1.TokenInfo{
			Id:        tok.ID.String(),
			Name:      tok.Name,
			CreatedAt: timestamppb.New(tok.CreateTime),
			ExpiresAt: timestamppb.New(tok.ExpiresAt),
			IsActive:  isActive,
		}

		if tok.Description != "" {
			protoToken.Description = &tok.Description
		}

		protoTokens = append(protoTokens, protoToken)
	}

	return connect.NewResponse(&v1.ListTokensResponse{
		Tokens: protoTokens,
	}), nil
}

func (h *AuthHandler) RevokeToken(ctx context.Context, req *connect.Request[v1.RevokeTokenRequest]) (*connect.Response[v1.RevokeTokenResponse], error) {
	identity := auth.FromContext(ctx)
	if identity == nil || !identity.IsAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("admin privileges required"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid ID format: %w", err)))
	}

	deleted, err := h.db.Token.Delete().Where(token.IDEQ(id)).Exec(ctx)
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to delete token: %w", err))
	}

	if deleted == 0 {
		return nil, apiError(connect.NewError(connect.CodeNotFound, fmt.Errorf("token not found")))
	}

	return connect.NewResponse(&v1.RevokeTokenResponse{}), nil
}

func (h *AuthHandler) ExchangeSetupCode(ctx context.Context, req *connect.Request[v1.ExchangeSetupCodeRequest]) (*connect.Response[v1.ExchangeSetupCodeResponse], error) {
	if req.Msg.SetupCode == "" {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("setup_code is required")))
	}

	setupCode := h.tokenProvider.ConsumeSetupCode(req.Msg.SetupCode)
	if setupCode == nil {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("invalid or expired setup code"))
	}

	exists, err := h.db.Token.Query().Where(token.NameEQ(setupCode.TokenName)).Exist(ctx)
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to check if token exists: %w", err))
	}
	if exists {
		return nil, apiError(connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("token with name %q already exists", setupCode.TokenName)))
	}

	plaintext, hash, err := h.tokenProvider.GenerateToken()
	if err != nil {
		return nil, apiError(fmt.Errorf("failed to generate token: %w", err))
	}

	expiresAt := time.Now().Add(setupCode.TokenExpiry)

	_, err = h.db.Token.Create().
		SetName(setupCode.TokenName).
		SetType(types.TokenTypeAPIToken).
		SetTokenHash(hash).
		SetExpiresAt(expiresAt).
		Save(ctx)

	if err != nil {
		return nil, apiError(fmt.Errorf("failed to save token: %w", err))
	}

	return connect.NewResponse(&v1.ExchangeSetupCodeResponse{
		Token:     plaintext,
		ExpiresAt: timestamppb.New(expiresAt),
		Name:      setupCode.TokenName,
	}), nil
}
