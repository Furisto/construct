package agent

import (
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/model"
)

type CompactionConfig struct {
	TMax          int64
	TRetained     int64
	SummaryBudget int64
	ContextWindow int64
}

func DefaultCompactionConfig(contextWindow int64) *CompactionConfig {
	return &CompactionConfig{
		TMax:          int64(float64(contextWindow) * 0.8),
		TRetained:     int64(float64(contextWindow) * 0.5),
		SummaryBudget: 4096,
		ContextWindow: contextWindow,
	}
}

type CompactionService struct {
	memory *memory.Client
	config *CompactionConfig
}

func NewCompactionService(memory *memory.Client, config *CompactionConfig) *CompactionService {
	return &CompactionService{
		memory: memory,
		config: config,
	}
}

func (s *CompactionService) ShouldCompact(lastUsage model.Usage) bool {
	totalTokens := lastUsage.InputTokens + lastUsage.OutputTokens +
		lastUsage.CacheReadTokens + lastUsage.CacheWriteTokens
	return totalTokens > s.config.TMax
}

func (s *CompactionService) FindTruncationPoint(messages []*memory.Message, anchorMessageID *string) int {
	if len(messages) == 0 {
		return -1
	}

	var tokenCount int64
	truncationPoint := len(messages)

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]

		if anchorMessageID != nil && msg.ID.String() == *anchorMessageID {
			break
		}

		if msg.Usage != nil {
			tokenCount += msg.Usage.InputTokens + msg.Usage.OutputTokens
		}

		if tokenCount <= s.config.TRetained {
			truncationPoint = i
		} else {
			break
		}
	}

	return truncationPoint
}
