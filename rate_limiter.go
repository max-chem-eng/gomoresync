package gomoresync

import (
	"context"
	"errors"
	"time"
)

// RateLimiter controls how frequently events are allowed to happen.
type RateLimiter interface {
	Wait(ctx context.Context) error
	Allow() bool
}

// TokenBucketLimiter implements a token bucket algorithm using channels.
type TokenBucketLimiter struct {
	capacity     int           // maximum tokens
	tokens       chan struct{} // channel to represent tokens
	fillInterval time.Duration // how often to add 1 token
	closed       chan struct{}
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
		tokens:       make(chan struct{}, capacity),
		fillInterval: fillInterval,
		closed:       make(chan struct{}),
	}

	// Fill the tokens channel to capacity
	for i := 0; i < capacity; i++ {
		tb.tokens <- struct{}{}
	}

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
		case <-tb.closed:
			return
		case <-ticker.C:
			select {
			case tb.tokens <- struct{}{}:
			default:
				// tokens channel is full, do nothing
			}
		}
	}
}

// Wait blocks until a token becomes available or ctx is canceled.
func (tb *TokenBucketLimiter) Wait(ctx context.Context) error {
	select {
	case <-tb.closed:
		return errors.New("token bucket limiter is closed")
	case <-ctx.Done():
		return ctx.Err()
	case <-tb.tokens:
		return nil
	}
}

// Allow is a non-blocking check if a token is available. Returns true if token is acquired.
func (tb *TokenBucketLimiter) Allow() bool {
	select {
	case <-tb.closed:
		return false
	case <-tb.tokens:
		return true
	default:
		return false
	}
}

// Close stops the refill goroutine and marks the limiter as closed.
func (tb *TokenBucketLimiter) Close() {
	close(tb.closed)
}
