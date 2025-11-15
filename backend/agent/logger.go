package agent

import (
	"fmt"
	"log/slog"
	"time"
)

// Log key constants for consistency across the codebase
const (
	// Task and agent identifiers
	KeyTaskID    = "task_id"
	KeyAgentID   = "agent_id"
	KeyMessageID = "message_id"

	// Model and provider information
	KeyModel         = "model"
	KeyProvider      = "provider"
	KeyModelProvider = "model_provider"

	// Timing and performance
	KeyDuration   = "duration_ms"
	KeyStartTime  = "start_time"
	KeyEndTime    = "end_time"
	KeyLatency    = "latency_ms"
	KeyThroughput = "throughput"

	// Token and cost tracking
	KeyInputTokens      = "input_tokens"
	KeyOutputTokens     = "output_tokens"
	KeyTotalTokens      = "total_tokens"
	KeyCacheWriteTokens = "cache_write_tokens"
	KeyCacheReadTokens  = "cache_read_tokens"
	KeyCost             = "cost"
	KeyTokensPerSecond  = "tokens_per_second"
	KeyCacheHitRatio    = "cache_hit_ratio"

	// Tool and execution information
	KeyTool         = "tool"
	KeyToolCount    = "tool_count"
	KeyToolName     = "tool_name"
	KeyToolDuration = "tool_duration_ms"
	KeyToolResult   = "tool_result"
	KeyToolStats    = "tool_stats"

	// Message and queue information
	KeyMessageCount     = "message_count"
	KeyProcessedCount   = "processed_count"
	KeyUnprocessedCount = "unprocessed_count"
	KeyQueueDepth       = "queue_depth"
	KeyActiveTaskCount  = "active_task_count"
	KeyPhase            = "phase"
	KeyNextPhase        = "next_phase"

	// Configuration and environment
	KeyConcurrency     = "concurrency"
	KeyToolsRegistered = "tools_registered"
	KeyListenerAddress = "listener_address"
	KeyListenerType    = "listener_type"

	// Error and retry information
	KeyRetryable  = "retryable"
	KeyRetryAfter = "retry_after_ms"
	KeyRetryCount = "retry_count"
	KeyErrorType  = "error_type"
	KeyError      = "error"

	// Operations and components
	KeyOperation = "operation"
	KeyComponent = "component"
)

// LoggerConfig holds logging configuration
type LoggerConfig struct {
	Level         slog.Level
	VerboseTools  bool
	VerboseModels bool
}

// DefaultLoggerConfig returns default logging configuration
func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:         slog.LevelInfo,
		VerboseTools:  false,
		VerboseModels: false,
	}
}

// LogOperationStart logs the start of a significant operation
func LogOperationStart(logger *slog.Logger, operation string, keyValues ...any) {
	args := append([]any{KeyOperation, operation}, keyValues...)
	logger.Debug(fmt.Sprintf("%s started", operation), args...)
}

// LogOperationEnd logs the end of a significant operation
func LogOperationEnd(logger *slog.Logger, operation string, start time.Time, keyValues ...any) {
	args := append(
		[]any{
			KeyOperation, operation,
			KeyDuration, time.Since(start).Milliseconds(),
		},
		keyValues...,
	)
	logger.Debug(fmt.Sprintf("%s completed", operation), args...)
}

// LogTokenUsage logs token usage from an API call
func LogTokenUsage(logger *slog.Logger, level slog.Level, inputTokens, outputTokens, cacheWrite, cacheRead int64, cost float64, duration time.Duration, keyValues ...any) {
	totalTokens := inputTokens + outputTokens + cacheWrite + cacheRead
	tokensPerSec := 0.0
	if duration.Seconds() > 0 {
		tokensPerSec = float64(outputTokens) / duration.Seconds()
	}
	cacheHitRatio := 0.0
	if inputTokens+cacheRead > 0 {
		cacheHitRatio = float64(cacheRead) / float64(inputTokens+cacheRead)
	}

	args := append(
		[]any{
			KeyInputTokens, inputTokens,
			KeyOutputTokens, outputTokens,
			KeyTotalTokens, totalTokens,
			KeyCacheWriteTokens, cacheWrite,
			KeyCacheReadTokens, cacheRead,
			KeyCost, fmt.Sprintf("$%.6f", cost),
			KeyTokensPerSecond, fmt.Sprintf("%.2f", tokensPerSec),
			KeyCacheHitRatio, fmt.Sprintf("%.1f%%", cacheHitRatio*100),
			KeyDuration, duration.Milliseconds(),
		},
		keyValues...,
	)
	logger.Log(nil, level, "token usage", args...)
}

// LogPhaseTransition logs task phase transitions
func LogPhaseTransition(logger *slog.Logger, taskID string, from, to string, keyValues ...any) {
	args := append(
		[]any{
			KeyTaskID, taskID,
			"from_phase", from,
			"to_phase", to,
		},
		keyValues...,
	)
	logger.Debug("task phase transition", args...)
}

// LogComponentStartup logs component initialization
func LogComponentStartup(logger *slog.Logger, component string, keyValues ...any) {
	args := append(
		[]any{KeyComponent, component},
		keyValues...,
	)
	logger.Info(fmt.Sprintf("%s initializing", component), args...)
}

// LogComponentShutdown logs component shutdown
func LogComponentShutdown(logger *slog.Logger, component string, shutdownStart time.Time, keyValues ...any) {
	args := append(
		[]any{
			KeyComponent, component,
			KeyDuration, time.Since(shutdownStart).Milliseconds(),
		},
		keyValues...,
	)
	logger.Info(fmt.Sprintf("%s shutdown complete", component), args...)
}

// LogError logs an error with structured context
func LogError(logger *slog.Logger, operation string, err error, keyValues ...any) {
	args := append(
		[]any{
			KeyOperation, operation,
			KeyError, err.Error(),
		},
		keyValues...,
	)
	logger.Error(fmt.Sprintf("failed: %s", operation), args...)
}
