package resilience

import (
	"sync"
	"time"
)

type CircuitBreaker struct {
	mu               sync.RWMutex
	provider         string
	failureThreshold int
	resetTimeout     time.Duration

	consecutiveFailures int
	lastFailureTime     time.Time
	state               CircuitState
	reopenAt            time.Time
}

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func NewCircuitBreaker(provider string, threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		provider:         provider,
		failureThreshold: threshold,
		resetTimeout:     resetTimeout,
		state:            CircuitClosed,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitOpen:
		if time.Now().After(cb.reopenAt) {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = CircuitHalfOpen
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		return false
	default:
		return true
	}
}

func (cb *CircuitBreaker) RecordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err == nil {
		cb.consecutiveFailures = 0
		if cb.state == CircuitHalfOpen {
			cb.state = CircuitClosed
		}
		return
	}

	cb.consecutiveFailures++
	cb.lastFailureTime = time.Now()

	if cb.consecutiveFailures >= cb.failureThreshold {
		cb.state = CircuitOpen
		cb.reopenAt = time.Now().Add(cb.resetTimeout)
	}
}
