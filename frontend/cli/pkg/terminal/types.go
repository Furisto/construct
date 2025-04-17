package terminal

type messageType int

const (
	MessageTypeUser messageType = iota
	MessageTypeAssistantText
	MessageTypeAssistantTool
	MessageTypeAssistantTyping
)

type message interface {
	Type() messageType
}

type userMessage struct {
	content string
}

func (m *userMessage) Type() messageType {
	return MessageTypeUser
}

type assistantTextMessage struct {
	content string
}

func (m *assistantTextMessage) Type() messageType {
	return MessageTypeAssistantText
}

type assistantToolMessage struct {
	callID  string
	name    string
	input   string
	output  string // Can store result or error
	isError bool   // Flag to indicate if output is an error
}

func (m *assistantToolMessage) Type() messageType {
	return MessageTypeAssistantTool
}
