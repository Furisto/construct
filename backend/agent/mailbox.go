package agent

import (
	"sync"
)

type TaskQueue struct {
	messages []string
}

type Mailbox struct {
	mu     sync.RWMutex
	queues map[string]*TaskQueue
}

func NewMailbox() *Mailbox {
	return &Mailbox{
		queues: make(map[string]*TaskQueue),
	}
}

func (m *Mailbox) Enqueue(taskID string, message string) {
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

func (m *Mailbox) Dequeue(taskID string) []string {
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