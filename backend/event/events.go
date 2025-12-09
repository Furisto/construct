package event

import "github.com/google/uuid"

type TaskEvent struct {
	TaskID uuid.UUID
}

func (TaskEvent) Event() {}

type TaskSuspendedEvent struct {
	TaskID uuid.UUID
}

func (TaskSuspendedEvent) Event() {}

type MessageEvent struct {
	MessageID uuid.UUID
	TaskID    uuid.UUID
}

func (MessageEvent) Event() {}

type ErrorEvent struct {
	TaskID uuid.UUID
	Error  error
}

func (ErrorEvent) Event() {}
