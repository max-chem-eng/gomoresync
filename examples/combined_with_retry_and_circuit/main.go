package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func main() {
	rand.Seed(time.Now().UnixNano()) // for jitter backoff randomness

	// 1. Create a top-level context that we’ll cancel on SIGINT/SIGTERM for graceful shutdown
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleInterrupt(cancel)

	// 2. Initialize concurrency primitives

	// 2a. Rate Limiter: capacity=5 tokens, refill 1 token per second
	limiter, err := gomoresync.NewTokenBucketLimiter(5, 1*time.Second)
	if err != nil {
		log.Fatal("failed to create rate limiter:", err)
	}
	defer limiter.Close()

	// 2b. Worker Pool: max 10 workers, buffered queue of 20
	pool, err := gomoresync.NewPool(
		gomoresync.WithMaxWorkers(5),
		gomoresync.WithBufferSize(20),
		gomoresync.WithSubmitBehavior(gomoresync.SubmitBlock),
		gomoresync.WithErrorAggregator(&gomoresync.AllErrorAggregator{}),
	)
	if err != nil {
		log.Fatal("failed to create worker pool:", err)
	}

	// 2c. Circuit Breaker for external service calls
	cb := gomoresync.NewCircuitBreaker(gomoresync.CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 1,
		OpenTimeout:      10 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	})

	// 2d. Prepare a RetryConfig with exponential backoff + jitter
	retryCfg := gomoresync.RetryConfig{
		MaxRetries:    5,
		LastErrorOnly: true,
		Backoff: gomoresync.JitterBackoff{
			Delegate: gomoresync.ExponentialBackoff{
				BaseDelay: 500 * time.Millisecond,
				Factor:    2.0,
				MaxDelay:  5 * time.Second,
			},
			JitterFactor: 0.3,
		},
	}

	// 3. Set up an HTTP handler that uses these concurrency constructs
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the client closes the connection, r.Context() will be canceled.
		// For demonstration, let's create a derived context from rootCtx,
		// so the task continues even if the client disconnects:
		taskCtx, taskCancel := context.WithTimeout(rootCtx, 15*time.Second)
		defer taskCancel()

		// Use the rate limiter first (blocking call)
		if err := limiter.Wait(r.Context()); err != nil {
			http.Error(w, fmt.Sprintf("Rate limited: %v", err), http.StatusTooManyRequests)
			return
		}

		// Submit the request processing task to our worker pool
		submitErr := pool.Submit(taskCtx, func(ctx context.Context) error {
			// This simulates calling an external service with circuit breaker + retry
			return circuitBreakerWithRetry(ctx, cb, retryCfg)
		})
		if submitErr != nil {
			http.Error(w, fmt.Sprintf("Cannot submit task: %v", submitErr), http.StatusServiceUnavailable)
			return
		}

		// We won't wait for the entire task to finish in the handler – returning async
		fmt.Fprintln(w, "Request accepted, processing asynchronously...")
	})

	mux := http.NewServeMux()
	mux.Handle("/", handler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// 4. Start the HTTP server in a goroutine
	serverErrCh := make(chan error, 1)
	go func() {
		log.Println("Starting server on :8080 ...")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
	}()

	// 5. Wait for either server errors or the context to be canceled
	select {
	case <-rootCtx.Done():
		log.Println("Shutting down gracefully...")
	case err := <-serverErrCh:
		log.Println("Server error:", err)
	}

	// Graceful shutdown of HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)

	// Make sure all tasks in the pool are finished
	poolErr := pool.Wait()
	if poolErr != nil {
		log.Printf("Worker pool completed with errors:\n%v\n", poolErr)
	} else {
		log.Println("All tasks completed successfully.")
	}
}

// handleInterrupt cancels the context on SIGINT/SIGTERM
func handleInterrupt(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancel()
}

// circuitBreakerWithRetry wraps an external call with circuit breaker + retry logic.
func circuitBreakerWithRetry(ctx context.Context, cb *gomoresync.CircuitBreaker, cfg gomoresync.RetryConfig) error {
	operation := func() error {
		log.Println("Attempting external call with circuit breaker and retry")
		return cb.Do(ctx, simulateExternalCall)
	}

	// Before calling the Retry function
	retryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use retryCtx when calling Retry
	err := gomoresync.Retry(retryCtx, operation, cfg)

	return err
}

// simulateExternalCall is a stand-in for a flaky external service.
// Fails 75% of the time, succeeds on every 4th call.
var (
	externalCallMutex sync.Mutex
	attemptCount      int
)

func simulateExternalCall() error {
	externalCallMutex.Lock()
	defer externalCallMutex.Unlock()

	attemptCount++
	if attemptCount%4 != 0 {
		// simulate a failure 75% of the time
		log.Printf("simulateExternalCall: attempt %d failed", attemptCount)
		return errors.New("external service failed")
	}
	// succeed occasionally
	time.Sleep(200 * time.Millisecond)
	log.Printf("simulateExternalCall: attempt %d succeeded", attemptCount)
	return nil
}
