//go:build darwin

package listener

import (
	"fmt"
	"net"
	"os"
	"strings"

	launchd "github.com/bored-engineer/go-launchd"
)

type LaunchdSocketProvider struct{}

func NewLaunchdSocketProvider() *LaunchdSocketProvider {
	return &LaunchdSocketProvider{}
}

func (p *LaunchdSocketProvider) Create() (net.Listener, error) {
	listener, err := launchd.Activate("Listeners")
	if err != nil {
		return nil, fmt.Errorf("failed to activate launchd socket: %w", err)
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
	return strings.HasPrefix(os.Getenv("XPC_SERVICE_NAME"), "sh.construct.daemon.")
}

func DetectProvider(httpAddress, unixSocket string) (Provider, error) {
	if unixSocket != "" {
		return NewUnixSocketProvider(unixSocket), nil
	}

	if httpAddress != "" {
		return NewTCPListenerProvider(httpAddress), nil
	}

	if IsLaunchdSocketActivation() {
		return NewLaunchdSocketProvider(), nil
	}

	return nil, fmt.Errorf("no valid listener has been detected. Specify either a unix socket, tcp address or use launchd socket activation")
}
