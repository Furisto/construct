package types

import "github.com/google/uuid"

type TaskSpec struct {
	AgentID uuid.UUID
}

type TaskStatus struct {
	Usage TaskUsage
}

type TaskUsage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheWriteTokens int64
	CacheReadTokens  int64
	Cost             float64
}
