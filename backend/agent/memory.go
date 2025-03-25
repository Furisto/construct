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

type FileMemory struct {
	FilePath string
}

func NewFileMemory(filePath string) *FileMemory {
	return &FileMemory{
		FilePath: filePath,
	}
}

func (m *FileMemory) Append(messages []model.Message) error {
	return nil
}

func (m *FileMemory) GetMessages() []model.Message {
	return nil
}

