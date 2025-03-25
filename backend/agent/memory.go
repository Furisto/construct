package agent

import "github.com/furisto/construct/backend/model"
type Memory interface {
	Append(messages []model.Message) error
}

type EphemeralMemory struct {
	Messages []model.Message
}

func NewEphemeralMemory() *EphemeralMemory {
	return &EphemeralMemory{
		Messages: []model.Message{},
	}
}

func (m *EphemeralMemory) Append(messages []model.Message) error {
	m.Messages = append(m.Messages, messages...)
	return nil
}

func (m *EphemeralMemory) GetMessages() []model.Message {
	return m.Messages
}
