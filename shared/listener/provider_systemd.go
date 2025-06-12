package listener

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

const systemdSocketFD = 3

type SystemdSocketProvider struct{}

func NewSystemdSocketProvider() *SystemdSocketProvider {
	return &SystemdSocketProvider{}
}

func (p *SystemdSocketProvider) Create() (net.Listener, error) {
	listenFds := os.Getenv("LISTEN_FDS")
	if listenFds == "" {
		return nil, fmt.Errorf("no LISTEN_FDS environment variable from systemd")
	}

	numFds, err := strconv.Atoi(listenFds)
	if err != nil {
		return nil, fmt.Errorf("invalid LISTEN_FDS value: %w", err)
	}

	if numFds < 1 {
		return nil, fmt.Errorf("no sockets passed from systemd")
	}

	if !isSocket(systemdSocketFD) {
		return nil, fmt.Errorf("file descriptor %d is not a socket", systemdSocketFD)
	}

	file := os.NewFile(systemdSocketFD, "systemd-socket")
	if file == nil {
		return nil, fmt.Errorf("no socket passed from systemd on FD %d", systemdSocketFD)
	}

	listener, err := net.FileListener(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener from systemd socket: %w", err)
	}

	return listener, nil
}

func (p *SystemdSocketProvider) Close() error {
	return nil
}

func (p *SystemdSocketProvider) ActivationType() string {
	return "systemd"
}

func IsSystemdSocketActivation() bool {
	if os.Getenv("LISTEN_FDS") != "" && os.Getenv("LISTEN_PID") != "" {
		listenPid := os.Getenv("LISTEN_PID")
		currentPid := strconv.Itoa(os.Getpid())
		if listenPid == currentPid {
			return true
		}
	}

	return false
}
