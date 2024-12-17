package gomoresync

import (
	"context"
	"errors"
	"sync"
	"time"
)

// RateLimiter controls how frequently events are allowed to happen.
type RateLimiter interface {
	Wait(ctx context.Context) error
	Allow() bool
}

// TokenBucketLimiter implements a token bucket algorithm.
type TokenBucketLimiter struct {
	capacity     int           // maximum tokens
	tokens       int           // current tokens
	fillInterval time.Duration // how often to add 1 token
	mu           sync.Mutex
	cond         *sync.Cond
	closed       bool
	stopCh       chan struct{}
}

// NewTokenBucketLimiter creates a token bucket that refills one token
// every fillInterval, up to 'capacity'.
func NewTokenBucketLimiter(capacity int, fillInterval time.Duration) (*TokenBucketLimiter, error) {
	if capacity <= 0 {
		return nil, errors.New("capacity must be > 0")
	}
	if fillInterval <= 0 {
		return nil, errors.New("fillInterval must be > 0")
	}

	tb := &TokenBucketLimiter{
		capacity:     capacity,
		tokens:       capacity, // start full
		fillInterval: fillInterval,
		stopCh:       make(chan struct{}),
	}
	tb.cond = sync.NewCond(&tb.mu)

	// Start a background goroutine to refill tokens
	go tb.refillTokens()
	return tb, nil
}

// refillTokens runs in the background, periodically adding tokens up to capacity.
func (tb *TokenBucketLimiter) refillTokens() {
	ticker := time.NewTicker(tb.fillInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tb.stopCh:
			return
		case <-ticker.C:
			tb.mu.Lock()
			if tb.closed {
				tb.mu.Unlock()
				return
			}
			if tb.tokens < tb.capacity {
				tb.tokens++
				tb.cond.Broadcast()
			}
			tb.mu.Unlock()
		}
	}
}

// Wait blocks until a token becomes available or ctx is canceled.
func (tb *TokenBucketLimiter) Wait(ctx context.Context) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for {
		// If the limiter was closed, return
		if tb.closed {
			return errors.New("token bucket limiter is closed")
		}

		if tb.tokens > 0 {
			tb.tokens--
			return nil
		}

		// No tokens available; need to wait.
		waitCh := make(chan struct{})
		go func() {
			tb.cond.Wait()
			close(waitCh)
		}()

		select {
		case <-ctx.Done():
			// Cancel the waiting goroutine
			tb.cond.Broadcast() // unblock any waiting goroutine
			return ctx.Err()
		case <-waitCh:
			// We were signaled; re-check loop condition
		}
	}
}

// Allow is a non-blocking check if a token is available. Returns true if token is acquired.
func (tb *TokenBucketLimiter) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.closed {
		return false
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// Close stops the refill goroutine and marks the limiter as closed.
func (tb *TokenBucketLimiter) Close() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if !tb.closed {
		tb.closed = true
		close(tb.stopCh)
		tb.cond.Broadcast()
	}
}
