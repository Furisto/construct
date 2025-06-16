package listener

import (
	"fmt"
	"net"
)

type TCPProvider struct {
	httpAddress string
}

var _ Provider = (*TCPProvider)(nil)

func NewTCPListenerProvider(httpAddress string) *TCPProvider {
	return &TCPProvider{
		httpAddress: httpAddress,
	}
}

func (p *TCPProvider) Create() (net.Listener, error) {
	listener, err := net.Listen("tcp", p.httpAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on tcp: %w", err)
	}

	return listener, nil
}

func (p *TCPProvider) Close() error {
	return nil
}

func (p *TCPProvider) ActivationType() string {
	return "tcp"
}
