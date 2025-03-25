package agent

import (
	"github.com/google/uuid"
)

type Task struct {
	ID uuid.UUID

	InputTokens      int64
	OutputTokens     int64
	CacheWriteTokens int64
	CacheReadTokens  int64

	LastUserMessage   uuid.UUID
	LastSystemMessage uuid.UUID
}
