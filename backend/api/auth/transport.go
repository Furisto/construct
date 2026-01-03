package auth

import "context"

type TransportType string

const (
	TransportUnix TransportType = "unix"
	TransportTCP  TransportType = "tcp"
)

type transportKey struct{}

func TransportFromContext(ctx context.Context) TransportType {
	transport, ok := ctx.Value(transportKey{}).(TransportType)
	if !ok {
		return ""
	}
	return transport
}

func WithTransport(ctx context.Context, transport TransportType) context.Context {
	return context.WithValue(ctx, transportKey{}, transport)
}
