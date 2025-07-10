package terminal

import (
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
)

type messageType int

const (
	MessageTypeUser messageType = iota
	MessageTypeAssistantText
	MessageTypeAssistantTool
	MessageTypeAssistantTyping
	MessageTypeSubmitReport
	MessageTypeError
)

type message interface {
	Type() messageType
	Timestamp() time.Time
}

type userMessage struct {
	content   string
	timestamp time.Time
}

func (m *userMessage) Type() messageType {
	return MessageTypeUser
}

func (m *userMessage) Timestamp() time.Time {
	return m.timestamp
}

type assistantTextMessage struct {
	content   string
	timestamp time.Time
}

func (m *assistantTextMessage) Type() messageType {
	return MessageTypeAssistantText
}

func (m *assistantTextMessage) Timestamp() time.Time {
	return m.timestamp
}

type assistantToolMessage struct {
	toolName   v1.ToolName
	arguments  map[string]string
	result     string
	error      string
	timestamp  time.Time
}

func (m *assistantToolMessage) Type() messageType {
	return MessageTypeAssistantTool
}

func (m *assistantToolMessage) Timestamp() time.Time {
	return m.timestamp
}

type submitReportMessage struct {
	summary       string
	completed     bool
	deliverables  []string
	nextSteps     string
	timestamp     time.Time
}

func (m *submitReportMessage) Type() messageType {
	return MessageTypeSubmitReport
}

func (m *submitReportMessage) Timestamp() time.Time {
	return m.timestamp
}

type errorMessage struct {
	content   string
	timestamp time.Time
}

func (m *errorMessage) Type() messageType {
	return MessageTypeError
}

func (m *errorMessage) Timestamp() time.Time {
	return m.timestamp
}

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
