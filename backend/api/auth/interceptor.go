package auth

import (
	"context"
	"fmt"
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

func (a *AuthInterceptor) Unary() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure

			if a.unauthenticatedPaths[procedure] {
				return next(ctx, req)
			}

			transport := TransportFromContext(ctx)
			if transport == TransportUnix {
				identity := &Identity{
					Subject:    "local-admin",
					AuthMethod: AuthMethodUnixSocket,
					IsAdmin:    true,
				}
				ctx = WithIdentity(ctx, identity)
				return next(ctx, req)
			}

			authHeader := req.Header().Get("Authorization")
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
			ctx = WithIdentity(ctx, identity)

			return next(ctx, req)
		}
	}
}

func (a *AuthInterceptor) Stream() connect.StreamInterceptorFunc {
	return func(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
		return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
			procedure := conn.Spec().Procedure

			if a.unauthenticatedPaths[procedure] {
				return next(ctx, conn)
			}

			transport := TransportFromContext(ctx)
			if transport == TransportUnix {
				identity := &Identity{
					Subject:    "local-admin",
					AuthMethod: AuthMethodUnixSocket,
					IsAdmin:    true,
				}
				ctx = WithIdentity(ctx, identity)
				return next(ctx, conn)
			}

			authHeader := conn.RequestHeader().Get("Authorization")
			if authHeader == "" {
				return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("missing authorization header"))
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid authorization format"))
			}

			tokenValue := parts[1]

			if !strings.HasPrefix(tokenValue, TokenPrefix) {
				return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token format"))
			}

			tokenHash := a.tokenProvider.HashToken(tokenValue)

			tok, err := a.db.Token.Query().
				Where(token.TokenHashEQ(tokenHash)).
				Where(token.ExpiresAtGT(time.Now())).
				First(ctx)

			if err != nil {
				if memory.IsNotFound(err) {
					return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid or expired token"))
				}
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to validate token: %w", err))
			}

			identity := &Identity{
				Subject:    tok.Name,
				AuthMethod: AuthMethodToken,
				IsAdmin:    false,
				ExpiresAt:  tok.ExpiresAt,
			}
			ctx = WithIdentity(ctx, identity)

			return next(ctx, conn)
		}
	}
}
