package listener

import (
	"net"
)

type Provider interface {
	Create() (net.Listener, error)
	Close() error
	ActivationType() string
}
