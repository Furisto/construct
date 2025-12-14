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

// Subtask status enum
type SubtaskStatus int

const (
	SubtaskStatusPending SubtaskStatus = iota
	SubtaskStatusRunning
	SubtaskStatusCompleted
	SubtaskStatusError
)

// SubtaskOutput tracks the output of a spawned subtask
type SubtaskOutput struct {
	ToolCallID string        // ID of the spawn_task tool call (used as key)
	TaskID     string        // ID of the spawned task (populated when result arrives)
	AgentName  string        // Name of the agent running the subtask
	Prompt     string        // The prompt/instructions given to the subtask
	Messages   []message     // Sliding window buffer of messages (max 10)
	Status     SubtaskStatus // Current status of the subtask
	Error      error         // Error if subscription failed
}

// MaxSubtaskMessages is the maximum number of messages to show in the sliding window
const MaxSubtaskMessages = 10

// AddMessage adds a message to the subtask output, maintaining the sliding window
func (s *SubtaskOutput) AddMessage(msg message) {
	s.Messages = append(s.Messages, msg)
	if len(s.Messages) > MaxSubtaskMessages {
		s.Messages = s.Messages[len(s.Messages)-MaxSubtaskMessages:]
	}
}

// Subtask-related commands and messages

// subtaskSubscribeCmd triggers subscription to a subtask
type subtaskSubscribeCmd struct {
	toolCallID string // ID of the spawn_task tool call
	taskID     string // ID of the subtask to subscribe to
}

// subtaskMessageMsg wraps a message from a subtask stream
type subtaskMessageMsg struct {
	toolCallID string      // ID of the spawn_task tool call to associate with
	message    *v1.Message // The message from the subtask
}

// subtaskCompletedMsg signals that a subtask has finished
type subtaskCompletedMsg struct {
	toolCallID string // ID of the spawn_task tool call
}

// subtaskErrorMsg signals that subscription to a subtask failed
type subtaskErrorMsg struct {
	toolCallID string // ID of the spawn_task tool call
	err        error  // The error that occurred
}
