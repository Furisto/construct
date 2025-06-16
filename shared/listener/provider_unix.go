package listener

import (
	"fmt"
	"net"
	"os"
)

type UnixSocketProvider struct {
	listener   net.Listener
	socketPath string
}

var _ Provider = (*UnixSocketProvider)(nil)

func NewUnixSocketProvider(socketPath string) *UnixSocketProvider {
	return &UnixSocketProvider{
		socketPath: socketPath,
	}
}

func (p *UnixSocketProvider) Create() (net.Listener, error) {
	_ = os.Remove(p.socketPath)
	listener, err := net.Listen("unix", p.socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on unix socket: %w", err)
	}

	p.listener = listener
	if err := os.Chmod(p.socketPath, 0666); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to set socket permissions: %w", err)
	}

	return listener, nil
}

func (p *UnixSocketProvider) Close() error {
	if p.listener != nil {
		p.listener.Close()
	}
	return os.Remove(p.socketPath)
}

func (p *UnixSocketProvider) ActivationType() string {
	return "unixsocket"
}
