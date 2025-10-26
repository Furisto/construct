package event

import "github.com/google/uuid"

type TaskEvent struct {
	TaskID uuid.UUID
}

func (TaskEvent) Event() {}

type MessageEvent struct {
	MessageID uuid.UUID
	TaskID    uuid.UUID
}

func (MessageEvent) Event() {}
