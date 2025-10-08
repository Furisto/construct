package terminal

import (
	"time"
)

type appState int

const (
	StateNormal appState = iota
	StateWaiting
	StateError
	StateHelp
)

type uiMode int

const (
	ModeInput uiMode = iota
	ModeScroll
)

type suspendTaskCmd struct{}
type sendMessageCmd struct {
	content string
}
type getTaskCmd struct {
	taskId string
}
type getModelCmd struct {
	modelId string
}
type listAgentsCmd struct{}

type taskUpdatedMsg struct{}

type Error struct {
	Error error
	Time  time.Time
}

func NewError(err error) *Error {
	return &Error{Error: err, Time: time.Now()}
}

func (m *Error) Type() messageType {
	return MessageTypeError
}

func (m *Error) Timestamp() time.Time {
	return m.Time
}
