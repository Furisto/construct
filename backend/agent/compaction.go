package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
	"github.com/furisto/construct/backend/prompt"
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
	memory        *memory.Client
	modelProvider model.ModelProvider
	modelName     string
	config        *CompactionConfig
}

func NewCompactionService(
	memory *memory.Client,
	modelProvider model.ModelProvider,
	modelName string,
	config *CompactionConfig,
) *CompactionService {
	return &CompactionService{
		memory:        memory,
		modelProvider: modelProvider,
		modelName:     modelName,
		config:        config,
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

func (s *CompactionService) SummarizeSpan(ctx context.Context, messages []*memory.Message) (*types.TaskSummary, error) {
	if len(messages) == 0 {
		return &types.TaskSummary{}, nil
	}

	conversationText := renderMessagesAsText(messages)

	userMessage := &model.Message{
		Source: model.MessageSourceUser,
		Content: []model.ContentBlock{
			&model.TextBlock{Text: conversationText},
		},
	}

	compactionTool := NewCompactionTool()

	response, err := s.modelProvider.InvokeModel(
		ctx,
		s.modelName,
		prompt.Compaction,
		[]*model.Message{userMessage},
		model.WithTools(compactionTool),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke model for summarization: %w", err)
	}

	for _, block := range response.Content {
		if toolCall, ok := block.(*model.ToolCallBlock); ok {
			if toolCall.Tool == CompactionToolName {
				if _, err := compactionTool.Run(ctx, nil, toolCall.Args); err != nil {
					return nil, fmt.Errorf("failed to process compaction tool call: %w", err)
				}
				return compactionTool.Result(), nil
			}
		}
	}

	return nil, fmt.Errorf("model did not call the %s tool", CompactionToolName)
}

func (s *CompactionService) MergeSummaries(existing, new *types.TaskSummary) *types.TaskSummary {
	if existing == nil {
		return new
	}
	if new == nil {
		return existing
	}

	merged := &types.TaskSummary{}

	if new.SessionIntent != "" {
		merged.SessionIntent = new.SessionIntent
	} else {
		merged.SessionIntent = existing.SessionIntent
	}

	merged.PlayByPlay = append(existing.PlayByPlay, new.PlayByPlay...)

	merged.ArtifactTrail = make(map[string]string)
	for path, desc := range existing.ArtifactTrail {
		merged.ArtifactTrail[path] = desc
	}
	for path, desc := range new.ArtifactTrail {
		merged.ArtifactTrail[path] = desc
	}

	merged.Decisions = append(existing.Decisions, new.Decisions...)

	breadcrumbSet := make(map[string]struct{})
	for _, b := range existing.Breadcrumbs {
		breadcrumbSet[b] = struct{}{}
	}
	for _, b := range new.Breadcrumbs {
		breadcrumbSet[b] = struct{}{}
	}
	merged.Breadcrumbs = make([]string, 0, len(breadcrumbSet))
	for b := range breadcrumbSet {
		merged.Breadcrumbs = append(merged.Breadcrumbs, b)
	}

	merged.PendingTasks = new.PendingTasks

	return merged
}

func (s *CompactionService) TrimToTokenBudget(summary *types.TaskSummary, budget int64) *types.TaskSummary {
	estimatedTokens := estimateSummaryTokens(summary)
	if estimatedTokens <= budget {
		return summary
	}

	trimmed := &types.TaskSummary{
		SessionIntent: summary.SessionIntent,
		PlayByPlay:    make([]string, len(summary.PlayByPlay)),
		ArtifactTrail: make(map[string]string),
		Decisions:     make([]types.Decision, len(summary.Decisions)),
		Breadcrumbs:   make([]string, len(summary.Breadcrumbs)),
		PendingTasks:  make([]string, len(summary.PendingTasks)),
	}

	copy(trimmed.PlayByPlay, summary.PlayByPlay)
	for k, v := range summary.ArtifactTrail {
		trimmed.ArtifactTrail[k] = v
	}
	copy(trimmed.Decisions, summary.Decisions)
	copy(trimmed.Breadcrumbs, summary.Breadcrumbs)
	copy(trimmed.PendingTasks, summary.PendingTasks)

	for estimateSummaryTokens(trimmed) > budget && len(trimmed.PlayByPlay) > 1 {
		trimmed.PlayByPlay = trimmed.PlayByPlay[1:]
	}

	for estimateSummaryTokens(trimmed) > budget && len(trimmed.ArtifactTrail) > 0 {
		for path := range trimmed.ArtifactTrail {
			trimmed.ArtifactTrail[path] = ""
			break
		}

		for path, desc := range trimmed.ArtifactTrail {
			if desc == "" {
				delete(trimmed.ArtifactTrail, path)
				break
			}
		}
	}

	return trimmed
}

func renderMessagesAsText(messages []*memory.Message) string {
	var builder strings.Builder

	for _, msg := range messages {
		var role string
		switch msg.Source {
		case types.MessageSourceUser:
			role = "User"
		case types.MessageSourceAssistant:
			role = "Assistant"
		case types.MessageSourceSystem:
			role = "System"
		}

		builder.WriteString(fmt.Sprintf("[%s]\n", role))

		for _, block := range msg.Content.Blocks {
			switch block.Kind {
			case types.MessageBlockKindText:
				builder.WriteString(block.Payload)
				builder.WriteString("\n")
			case types.MessageBlockKindCodeInterpreterCall:
				var toolCall model.ToolCallBlock
				if err := json.Unmarshal([]byte(block.Payload), &toolCall); err == nil {
					builder.WriteString(fmt.Sprintf("[Tool Call: %s]\n", toolCall.Tool))
				}
			case types.MessageBlockKindCodeInterpreterResult:
				builder.WriteString("[Tool Result]\n")
			case types.MessageBlockKindNativeToolCall:
				var toolCall model.ToolCallBlock
				if err := json.Unmarshal([]byte(block.Payload), &toolCall); err == nil {
					builder.WriteString(fmt.Sprintf("[Tool Call: %s]\n", toolCall.Tool))
				}
			case types.MessageBlockKindNativeToolResult:
				builder.WriteString("[Tool Result]\n")
			}
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

func estimateSummaryTokens(summary *types.TaskSummary) int64 {
	data, err := json.Marshal(summary)
	if err != nil {
		return 0
	}
	return int64(len(data) / 4)
}
