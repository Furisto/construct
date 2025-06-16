package listener

import (
	"net"
	"runtime"
	"syscall"
)

type Provider interface {
	Create() (net.Listener, error)
	Close() error
	ActivationType() string
}

func DetectProvider(httpAddress, unixSocket string) (Provider, error) {
	if unixSocket != "" {
		return NewUnixSocketProvider(unixSocket), nil
	}

	if httpAddress != "" {
		return NewTCPListenerProvider(httpAddress), nil
	}

	switch runtime.GOOS {
	case "darwin":
		if IsLaunchdSocketActivation() {
			return NewLaunchdSocketProvider(), nil
		}
	case "linux":
		if IsSystemdSocketActivation() {
			return NewSystemdSocketProvider(), nil
		}
	}

	return NewTCPListenerProvider("localhost:29333"), nil
}

func isSocket(fd uintptr) bool {
	var stat syscall.Stat_t
	err := syscall.Fstat(int(fd), &stat)
	if err != nil {
		return false
	}

	return stat.Mode&syscall.S_IFMT == syscall.S_IFSOCK
}
