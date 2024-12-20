# gomoresync

`gomoresync` is a Go library providing advanced concurrency patterns as a service. It includes well-tested utilities for:

- **Worker/Goroutine Pools**: Control concurrency with a configurable pool of workers.
- **Rate Limiters**: Token bucket-based rate limiting to throttle operations.
- **Circuit Breakers**: Avoid hitting failing dependencies continuously by using a circuit breaker pattern.
- **Structured Shutdown & Context Management**: Gracefully manage goroutines and ensure clean shutdown sequences.
- **Batching & Debouncing**: Batch multiple requests or debounce rapid-fire events.
- **Retry & Backoff Patterns**: Retry operations with configurable backoff strategies (fixed, exponential, jittered).

## Features

- **Context-Aware**: All primitives integrate with `context.Context` for cancellation and deadlines.
- **Configurable**: Use functional options to adjust buffer sizes, worker counts, error handling strategies, and more.
- **Error Aggregation**: Choose how multiple errors are aggregated (first error only, collect all errors, etc.).
- **Observers & Hooks**: Add custom observers to track task lifecycle events for monitoring and metrics.

## Installation

```bash
go get github.com/max-chem-eng/gomoresync
```
## Quick Start

Worker Pool:

```go
p, err := gomoresync.NewPool(
    gomoresync.WithMaxWorkers(5),
    gomoresync.WithBufferSize(10),
    gomoresync.WithErrorAggregator(&gomoresync.AllErrorAggregator{}),
)
if err != nil {
    log.Fatal(err)
}

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

for i := 0; i < 50; i++ {
    p.Submit(ctx, func(ctx context.Context) error {
        // Do some work here
        return nil
    })
}

if err := p.Wait(); err != nil {
    fmt.Println("Errors occurred:", err)
} else {
    fmt.Println("All tasks completed successfully.")
}
```
Rate Limiter:

```go
limiter, _ := gomoresync.NewTokenBucketLimiter(3, time.Second)
if err := limiter.Wait(context.Background()); err == nil {
    // proceed with operation
}
```

Circuit Breaker:

```go
cb := gomoresync.NewCircuitBreaker(gomoresync.CircuitBreakerConfig{
    FailureThreshold: 3,
    SuccessThreshold: 1,
    OpenTimeout:      5 * time.Second,
})

// Wrap calls
ctx := context.Background()
err := cb.Do(ctx, func() error {
    // Call external service
    return someOperation()
})
```

Retry & Backoff:

```go
cfg := gomoresync.RetryConfig{
    MaxRetries:    3,
    LastErrorOnly: false,
    Backoff: gomoresync.ExponentialBackoff{
        BaseDelay: 500 * time.Millisecond,
        Factor:    2.0,
        MaxDelay:  5 * time.Second,
    },
}

err := gomoresync.Retry(ctx, func() error {
    // flaky operation
    return doSomethingUnreliable()
}, cfg)
```

Check out the examples/ folder for more detailed usage scenarios

## Contributing

Contributions are welcome! Please open an issue or submit a PR for any new features, enhancements, or bug fixes.
