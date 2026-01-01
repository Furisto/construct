package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	v1connect "github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/token"
)

type AuthInterceptor struct {
	db                   *memory.Client
	tokenProvider        *TokenProvider
	unauthenticatedPaths map[string]bool
}

func NewAuthInterceptor(db *memory.Client, tokenProvider *TokenProvider) *AuthInterceptor {
	return &AuthInterceptor{
		db:            db,
		tokenProvider: tokenProvider,
		unauthenticatedPaths: map[string]bool{
			v1connect.AuthServiceExchangeSetupCodeProcedure: true,
		},
	}
}

func (a *AuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		identity, err := a.authenticate(ctx, req.Spec(), req.Header())
		if err != nil {
			return nil, connect.NewError(connect.CodeUnauthenticated, err)
		}
		if identity != nil {
			ctx = WithIdentity(ctx, identity)
		}

		return next(ctx, req)
	}
}

func (a *AuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, shc connect.StreamingHandlerConn) error {
		identity, err := a.authenticate(ctx, shc.Spec(), shc.RequestHeader())
		if err != nil {
			return connect.NewError(connect.CodeUnauthenticated, err)
		}
		ctx = WithIdentity(ctx, identity)

		return next(ctx, shc)
	}
}

func (a *AuthInterceptor) authenticate(ctx context.Context, spec connect.Spec, header http.Header) (*Identity, error) {
	procedure := spec.Procedure

	if a.unauthenticatedPaths[procedure] {
		return nil, nil
	}

	transport := TransportFromContext(ctx)
	if transport == TransportUnix {
		identity := &Identity{
			Subject:    "local-admin",
			AuthMethod: AuthMethodUnixSocket,
			IsAdmin:    true,
		}
		return identity, nil
	}
	
	authHeader := header.Get("Authorization")
	if authHeader == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing authorization header"))
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid authorization format"))
	}

	tokenValue := parts[1]

	if !strings.HasPrefix(tokenValue, TokenPrefix) {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token format"))
	}

	tokenHash := a.tokenProvider.HashToken(tokenValue)

	tok, err := a.db.Token.Query().
		Where(token.TokenHashEQ(tokenHash)).
		Where(token.ExpiresAtGT(time.Now())).
		First(ctx)

	if err != nil {
		if memory.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid or expired token"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to validate token: %w", err))
	}

	identity := &Identity{
		Subject:    tok.Name,
		AuthMethod: AuthMethodToken,
		IsAdmin:    false,
		ExpiresAt:  tok.ExpiresAt,
	}

	return identity, nil
}
