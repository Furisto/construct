package model

import (
	"context"
	_ "embed"
	"fmt"
)

type CondenserResult struct {
	AddedMessages   []*Message
	RemovedMessages []*Message
}

type Condenser interface {
	Condense(ctx context.Context, messages []*Message) (*CondenserResult, error)
}

// TruncationCondenser removes messages from the middle of the conversation
// when the context window is approaching its limit
type TruncationCondenser struct {
	// Maximum context window size
	ContextWindow int64
	// Percentage of context window to trigger truncation (e.g., 0.8 for 80%)
	TruncationRatio float64
	// Number of messages to preserve at the beginning and end
	PreserveCount int
	// Maximum percentage of messages to remove in one truncation
	MaxRemovalPercent float64
}

// NewTruncationCondenser creates a new TruncationCondenser with sensible defaults
func NewTruncationCondenser(contextWindow int64) *TruncationCondenser {
	return &TruncationCondenser{
		ContextWindow:     contextWindow,
		TruncationRatio:   0.8,
		PreserveCount:     2,
		MaxRemovalPercent: 0.5,
	}
}

func (c *TruncationCondenser) Condense(ctx context.Context, messages []*Message) (*CondenserResult, error) {
	if len(messages) == 0 {
		return &CondenserResult{}, nil
	}

	var lastModelMessage *Message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Source == MessageSourceModel {
			lastModelMessage = messages[i]
			break
		}
	}

	if lastModelMessage == nil {
		// No model message found, no truncation needed
		return &CondenserResult{}, nil
	}

	totalTokens := lastModelMessage.Usage.InputTokens +
		lastModelMessage.Usage.OutputTokens +
		lastModelMessage.Usage.CacheReadTokens +
		lastModelMessage.Usage.CacheWriteTokens

	truncationThreshold := int64(float64(c.ContextWindow) * c.TruncationRatio)
	if totalTokens < truncationThreshold {
		return &CondenserResult{}, nil
	}

	minMessagesForTruncation := (c.PreserveCount * 2) + 2
	if len(messages) < minMessagesForTruncation {
		return &CondenserResult{}, nil
	}

	startIdx := c.PreserveCount
	endIdx := len(messages) - c.PreserveCount
	eligibleMessages := messages[startIdx:endIdx]

	if len(eligibleMessages) == 0 {
		return &CondenserResult{}, nil
	}

	maxToRemove := int(float64(len(eligibleMessages)) * c.MaxRemovalPercent)
	if maxToRemove < 1 {
		maxToRemove = 1
	}

	// Remove messages from the middle of eligible range to preserve conversation flow
	removalStart := len(eligibleMessages)/2 - maxToRemove/2
	removalEnd := removalStart + maxToRemove

	if removalStart < 0 {
		removalStart = 0
	}
	if removalEnd > len(eligibleMessages) {
		removalEnd = len(eligibleMessages)
	}

	var removedMessages []*Message
	for i := removalStart; i < removalEnd; i++ {
		removedMessages = append(removedMessages, eligibleMessages[i])
	}

	return &CondenserResult{
		RemovedMessages: removedMessages,
		AddedMessages:   []*Message{},
	}, nil
}

var _ Condenser = &TruncationCondenser{}

// //go:embed ../prompt/summary.md
var summaryPrompt string

type SummarizationCondenser struct {
	modelProvider ModelProvider
}

func NewSummarizationCondenser(modelProvider ModelProvider) *SummarizationCondenser {
	return &SummarizationCondenser{
		modelProvider: modelProvider,
	}
}

func (c *SummarizationCondenser) Condense(ctx context.Context, model Model, messages []*Message) (map[*Message][]*Message, error) {
	var lastModelMessage *Message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Source == MessageSourceModel {
			lastModelMessage = messages[i]
			break
		}
	}

	if lastModelMessage == nil {
		return nil, fmt.Errorf("no model message found")
	}

	totalTokens := lastModelMessage.Usage.InputTokens + lastModelMessage.Usage.OutputTokens + lastModelMessage.Usage.CacheReadTokens + lastModelMessage.Usage.CacheWriteTokens
	if float64(totalTokens) < float64(model.ContextWindow)*0.8 {
		return nil, nil
	}

	return nil, nil
}
