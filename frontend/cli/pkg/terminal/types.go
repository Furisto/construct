package terminal

import (
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
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

type switchAgentCmd struct {
	agentId string
}

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

type SubtaskStatus int

const (
	SubtaskStatusPending SubtaskStatus = iota
	SubtaskStatusRunning
	SubtaskStatusCompleted
	SubtaskStatusError
)

type SubtaskOutput struct {
	ToolCallID   string
	TaskID       string
	AgentName    string
	Prompt       string
	Messages     []message
	Status       SubtaskStatus
	Error        error
	CreatedAt    time.Time
	LastActivity time.Time
}

const MaxSubtaskMessages = 10

const SubtaskStaleTimeout = 5 * time.Minute

func (s *SubtaskOutput) AddMessage(msg message) {
	s.Messages = append(s.Messages, msg)
	if len(s.Messages) > MaxSubtaskMessages {
		s.Messages = s.Messages[len(s.Messages)-MaxSubtaskMessages:]
	}
	s.LastActivity = time.Now()
}

func (s *SubtaskOutput) IsStale() bool {
	if s.Status == SubtaskStatusCompleted {
		return true
	}
	if s.Status == SubtaskStatusError {
		return time.Since(s.LastActivity) > 30*time.Second
	}
	return time.Since(s.LastActivity) > SubtaskStaleTimeout
}

type subtaskSubscribeCmd struct {
	toolCallID string
	taskID     string
}

type subtaskMessageMsg struct {
	toolCallID string
	message    *v1.Message
}

type subtaskCompletedMsg struct {
	toolCallID string
}

type subtaskErrorMsg struct {
	toolCallID string
	err        error
}

type subtaskCleanupTickMsg struct{}
