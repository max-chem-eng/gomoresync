package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func main() {
	// Create a token bucket limiter: capacity=3, refill one token every second
	limiter, err := gomoresync.NewTokenBucketLimiter(3, 1*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	defer limiter.Close()

	// Create a pool for processing tasks
	pool, err := gomoresync.NewPool(
		gomoresync.WithMaxWorkers(3),
		gomoresync.WithBufferSize(10),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 10; i++ {
		i := i
		// Enforce rate limit before submitting to the pool
		go func() {
			if err := limiter.Wait(ctx); err != nil {
				fmt.Printf("Task %d was not allowed: %v\n", i, err)
				return
			}
			err := pool.Submit(ctx, func(ctx context.Context) error {
				time.Sleep(500 * time.Millisecond)
				fmt.Printf("Task %d processed under rate limiter.\n", i)
				return nil
			})
			if err != nil {
				fmt.Printf("Submit error for task %d: %v\n", i, err)
			}
		}()
	}

	// Wait for all tasks
	if err := pool.Wait(); err != nil {
		fmt.Println("Errors from pool:", err)
	} else {
		fmt.Println("All tasks completed successfully under rate limiting.")
	}
}
