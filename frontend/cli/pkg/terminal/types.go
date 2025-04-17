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
