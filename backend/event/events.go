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

type TaskReconciliationEvent struct {
	TaskID uuid.UUID
}

func (TaskReconciliationEvent) Event() {}

type MessageEvent struct {
	MessageID uuid.UUID
	TaskID    uuid.UUID
}

func (MessageEvent) Event() {}

type DeltaMessageEvent struct {
	MessageID uuid.UUID
	TaskID    uuid.UUID
	Content   string
}

func (DeltaMessageEvent) Event() {}

type ErrorEvent struct {
	TaskID uuid.UUID
	Error  error
}

func (ErrorEvent) Event() {}
