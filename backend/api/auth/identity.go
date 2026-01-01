package auth

import (
	"context"
	"time"
)

type AuthMethod int

const (
	AuthMethodUnspecified AuthMethod = iota
	AuthMethodUnixSocket
	AuthMethodToken
)

func (a AuthMethod) String() string {
	switch a {
	case AuthMethodUnixSocket:
		return "unix_socket"
	case AuthMethodToken:
		return "token"
	default:
		return "unspecified"
	}
}

type Identity struct {
	Subject    string
	AuthMethod AuthMethod
	IsAdmin    bool
	ExpiresAt  time.Time
}

type identityKey struct{}

func FromContext(ctx context.Context) *Identity {
	identity, ok := ctx.Value(identityKey{}).(*Identity)
	if !ok {
		return nil
	}
	return identity
}

func WithIdentity(ctx context.Context, identity *Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}
