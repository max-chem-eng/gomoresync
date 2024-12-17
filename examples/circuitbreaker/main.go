package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func main() {
	cb := gomoresync.NewCircuitBreaker(gomoresync.CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 1,
		OpenTimeout:      3 * time.Second,
		HalfOpenMaxCalls: 1,
	})

	// This simulates an operation that fails initially, then succeeds.
	var failCount int
	operation := func() error {
		failCount++
		if failCount <= 4 {
			return errors.New("simulated failure")
		}
		return nil
	}

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		err := cb.Do(ctx, operation)
		if err != nil {
			fmt.Printf("Attempt %d failed. CB State=%s Error=%v\n", i, cb.State(), err)
		} else {
			fmt.Printf("Attempt %d succeeded. CB State=%s\n", i, cb.State())
		}
	}
	log.Printf("Final circuit breaker state: %s\n", cb.State())
}
