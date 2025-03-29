package agent

import (
	"github.com/google/uuid"
	"sync"
)

type TaskQueue struct {
	messages []string
}

type Mailbox struct {
	mu     sync.RWMutex
	queues map[uuid.UUID]*TaskQueue
}

func NewMailbox() *Mailbox {
	return &Mailbox{
		queues: make(map[uuid.UUID]*TaskQueue),
	}
}

func (m *Mailbox) Enqueue(taskID uuid.UUID, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[taskID]
	if !exists {
		queue = &TaskQueue{
			messages: []string{},
		}
		m.queues[taskID] = queue
	}

	queue.messages = append(queue.messages, message)
}

func (m *Mailbox) Dequeue(taskID uuid.UUID) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, exists := m.queues[taskID]
	if !exists {
		return []string{}
	}

	messages := queue.messages
	delete(m.queues, taskID)

	return messages
}
