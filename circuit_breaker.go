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
	state  int32 // atomic state
	mu     sync.Mutex
	// counters
	failCount    int
	successCount int
	// for open -> halfOpen
	lastOpenTime time.Time
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
		cfg.OpenTimeout = 10 * time.Second
	}
	if cfg.HalfOpenMaxCalls <= 0 {
		cfg.HalfOpenMaxCalls = 1
	}

	return &CircuitBreaker{
		config: cfg,
		state:  int32(StateClosed),
	}
}

// Do attempts the operation if the circuit breaker allows it.
// - If the state is Open, the call is rejected until the timeout passes.
// - If HalfOpen, only a limited number of calls are allowed concurrently.
func (cb *CircuitBreaker) Do(ctx context.Context, operation func() error) error {
	state := cb.currentState()

	switch state {
	case StateOpen:
		// Check if we can move to HalfOpen
		if time.Since(cb.lastOpenTime) >= cb.config.OpenTimeout {
			cb.transitionTo(StateHalfOpen)
		} else {
			return errors.New("circuit breaker is open")
		}
	case StateHalfOpen:
		// We could limit concurrency or calls here. For simplicity, if HalfOpenMaxCalls is > 1,
		// you might track the current half-open calls.
		// We'll do a quick concurrency guard:
		cb.mu.Lock()
		if cb.failCount+cb.successCount >= cb.config.HalfOpenMaxCalls {
			cb.mu.Unlock()
			return errors.New("circuit breaker half-open: max calls reached")
		}
		cb.mu.Unlock()
	}

	// Perform the operation
	err := operation()
	if err != nil {
		cb.onFailure()
		return err
	}
	cb.onSuccess()
	return nil
}

func (cb *CircuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failCount++
	cb.successCount = 0

	// If in HalfOpen, transition immediately to Open if fail
	if cb.currentState() == StateHalfOpen {
		cb.transitionTo(StateOpen)
		cb.lastOpenTime = time.Now()
		return
	}

	// If Closed, check threshold
	if cb.currentState() == StateClosed && cb.failCount >= cb.config.FailureThreshold {
		cb.transitionTo(StateOpen)
		cb.lastOpenTime = time.Now()
	}
}

func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successCount++
	// If in HalfOpen, we might close the breaker after enough successes
	if cb.currentState() == StateHalfOpen {
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.transitionTo(StateClosed)
			cb.failCount = 0
			cb.successCount = 0
		}
		return
	}

	// If in Closed, reset counters
	if cb.currentState() == StateClosed {
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
