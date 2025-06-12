package listener

import (
	"fmt"
	"net"
	"os"
)

const launchdSocketFD = 3

type LaunchdSocketProvider struct{}

func NewLaunchdSocketProvider() *LaunchdSocketProvider {
	return &LaunchdSocketProvider{}
}

func (p *LaunchdSocketProvider) Create() (net.Listener, error) {
	if !isSocket(launchdSocketFD) {
		return nil, fmt.Errorf("file descriptor %d is not a socket", launchdSocketFD)
	}

	file := os.NewFile(launchdSocketFD, "launchd-socket")
	if file == nil {
		return nil, fmt.Errorf("no socket passed from launchd on FD %d", launchdSocketFD)
	}

	listener, err := net.FileListener(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener from launchd socket: %w", err)
	}

	return listener, nil
}

func (p *LaunchdSocketProvider) Close() error {
	return nil
}

func (p *LaunchdSocketProvider) ActivationType() string {
	return "launchd"
}

func IsLaunchdSocketActivation() bool {
	return os.Getenv("LAUNCH_DAEMON_SOCKET_NAME") != ""
}
