package gomoresync

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreakerState represents the breaker’s current state.
type CircuitBreakerState int32

const (
	StateClosed CircuitBreakerState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

// CircuitBreakerConfig holds configuration for CircuitBreaker.
type CircuitBreakerConfig struct {
	FailureThreshold int           // Number of consecutive failures before opening
	SuccessThreshold int           // Number of consecutive successes in HalfOpen before closing
	OpenTimeout      time.Duration // How long to stay open before transitioning to HalfOpen
	HalfOpenMaxCalls int           // Max calls allowed in HalfOpen state (often 1)
}

// CircuitBreaker implements a simplistic circuit breaker pattern.
type CircuitBreaker struct {
	config CircuitBreakerConfig

	state int32 // atomic for StateClosed, StateOpen, StateHalfOpen

	mu           sync.Mutex
	failCount    int
	successCount int

	lastOpenTime time.Time

	// halfOpenActive tracks how many calls are currently allowed in half-open
	halfOpenActive int32
}

// NewCircuitBreaker creates a new CircuitBreaker with the specified config.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 1
	}
	if cfg.OpenTimeout <= 0 {
		cfg.OpenTimeout = 5 * time.Second
	}
	if cfg.HalfOpenMaxCalls <= 0 {
		cfg.HalfOpenMaxCalls = 1
	}

	cb := &CircuitBreaker{
		config: cfg,
		state:  int32(StateClosed),
	}
	return cb
}

// Do attempts the operation if the circuit breaker allows it.
// - If the state is Open, the call is rejected until the timeout passes.
// - If HalfOpen, only a limited number of calls are allowed concurrently.
func (cb *CircuitBreaker) Do(ctx context.Context, operation func() error) error {
	state := cb.currentState()

	switch state {
	case StateOpen:
		// Check if OpenTimeout has elapsed
		cb.mu.Lock()
		if time.Since(cb.lastOpenTime) >= cb.config.OpenTimeout {
			cb.transitionTo(StateHalfOpen)
		} else {
			cb.mu.Unlock()
			return errors.New("circuit breaker is open")
		}
		cb.mu.Unlock()

	case StateHalfOpen:
		// Safely enforce concurrency limit in HalfOpen
		active := atomic.LoadInt32(&cb.halfOpenActive)
		if active >= int32(cb.config.HalfOpenMaxCalls) {
			return errors.New("circuit breaker half-open: max calls reached")
		}
		atomic.AddInt32(&cb.halfOpenActive, 1)
		defer atomic.AddInt32(&cb.halfOpenActive, -1)
	}

	err := operation()
	if err == nil {
		cb.onSuccess()
		return nil
	}
	cb.onFailure()
	return err
}

func (cb *CircuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failCount++
	cb.successCount = 0
	curr := cb.currentState()

	if curr == StateHalfOpen {
		// Immediately transition back to open
		cb.transitionTo(StateOpen)
		cb.lastOpenTime = time.Now()
	} else if curr == StateClosed && cb.failCount >= cb.config.FailureThreshold {
		cb.transitionTo(StateOpen)
		cb.lastOpenTime = time.Now()
	}
}

func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successCount++

	if cb.currentState() == StateHalfOpen {
		// If success threshold reached, close breaker
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.transitionTo(StateClosed)
			cb.failCount = 0
			cb.successCount = 0
		}
	} else if cb.currentState() == StateClosed {
		// Normal closed state, reset failures
		cb.failCount = 0
	}
}

func (cb *CircuitBreaker) transitionTo(newState CircuitBreakerState) {
	atomic.StoreInt32(&cb.state, int32(newState))
}

func (cb *CircuitBreaker) currentState() CircuitBreakerState {
	return CircuitBreakerState(atomic.LoadInt32(&cb.state))
}

// State returns the breaker’s current state in a threadsafe manner.
func (cb *CircuitBreaker) State() CircuitBreakerState {
	return cb.currentState()
}
