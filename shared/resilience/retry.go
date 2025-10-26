package resilience

import (
	"context"
	"time"
)

type RetryConfig struct {
	MaxAttempts        uint
	InitialDelay       time.Duration
	MaxDelay           time.Duration
	UseProviderBackoff bool
	BackoffMultiplier  float64
}

type RetryHook interface {
	OnRetryAttempt(ctx context.Context, attempt uint, err error, nextDelay time.Duration)
	OnRetrySuccess(ctx context.Context, attempts uint, totalDuration time.Duration)
	OnRetryFailure(ctx context.Context, err error, attempts uint, totalDuration time.Duration)
}
