package gomoresync

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// BackoffStrategy is used to calculate the delay before the next retry attempt.
type BackoffStrategy interface {
	NextDelay(retryCount int) time.Duration
}

// FixedBackoff always returns the same delay.
type FixedBackoff struct {
	Delay time.Duration
}

func (f FixedBackoff) NextDelay(_ int) time.Duration {
	return f.Delay
}

// ExponentialBackoff grows exponentially by factor each retry, up to MaxDelay.
type ExponentialBackoff struct {
	BaseDelay time.Duration
	Factor    float64
	MaxDelay  time.Duration
}

func (e ExponentialBackoff) NextDelay(retryCount int) time.Duration {
	// delay = BaseDelay * Factor^(retryCount)
	pow := math.Pow(e.Factor, float64(retryCount))
	delay := time.Duration(float64(e.BaseDelay) * pow)
	if delay > e.MaxDelay && e.MaxDelay > 0 {
		delay = e.MaxDelay
	}
	return delay
}

// JitterBackoff wraps another BackoffStrategy and applies +/- random jitter.
type JitterBackoff struct {
	Delegate     BackoffStrategy
	JitterFactor float64 // 0.0 to 1.0
}

func (j JitterBackoff) NextDelay(retryCount int) time.Duration {
	baseDelay := j.Delegate.NextDelay(retryCount)
	if j.JitterFactor <= 0 {
		return baseDelay
	}
	mult := 1.0 + (rand.Float64()*2-1)*j.JitterFactor // range [1-JitterFactor, 1+JitterFactor]
	jittered := time.Duration(float64(baseDelay) * mult)
	if jittered < 0 {
		jittered = 0
	}
	return jittered
}

// RetryConfig configures the Retry function.
type RetryConfig struct {
	MaxRetries    int
	Backoff       BackoffStrategy
	LastErrorOnly bool // If true, returns only the last error; otherwise, wraps all errors
}

// Retry attempts the operation multiple times according to a backoff strategy.
func Retry(ctx context.Context, operation func() error, cfg RetryConfig) error {
	if cfg.Backoff == nil {
		cfg.Backoff = FixedBackoff{Delay: 1 * time.Second} // default
	}
	if cfg.MaxRetries < 1 {
		cfg.MaxRetries = 3 // default
	}

	var errs []error
	for i := 0; i <= cfg.MaxRetries; i++ {
		err := operation()
		if err == nil {
			return nil
		}
		errs = append(errs, err)

		// If this was our last attempt, break
		if i == cfg.MaxRetries {
			break
		}

		// Wait with backoff
		delay := cfg.Backoff.NextDelay(i)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			continue
		}
	}
	if cfg.LastErrorOnly {
		return errs[len(errs)-1]
	}
	return &AggregateError{Errors: errs}
}

// Example usage:
//
// func main() {
//   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//   defer cancel()
//
//   cfg := RetryConfig{
//       MaxRetries: 5,
//       Backoff: ExponentialBackoff{
//           BaseDelay: 500 * time.Millisecond,
//           Factor:    2.0,
//           MaxDelay:  5 * time.Second,
//       },
//       LastErrorOnly: false,
//   }
//
//   err := Retry(ctx, func() error {
//       // some failing operation
//       return errors.New("transient error")
//   }, cfg)
//   if err != nil {
//       fmt.Println("Operation failed after retries:", err)
//   }
// }
