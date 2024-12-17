package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func main() {
	// Create a worker pool
	pool, err := gomoresync.NewPool(
		gomoresync.WithMaxWorkers(5),
		gomoresync.WithBufferSize(10),
		gomoresync.WithSubmitBehavior(gomoresync.SubmitBlock),
		gomoresync.WithErrorAggregator(&gomoresync.AllErrorAggregator{}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create a rate limiter (capacity=3 tokens, refill one token every second)
	limiter, err := gomoresync.NewTokenBucketLimiter(3, 800*time.Millisecond)
	if err != nil {
		log.Fatal(err)
	}
	defer limiter.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Submit tasks in a rate-limited fashion
	for i := 0; i < 10; i++ {
		taskIndex := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := limiter.Wait(ctx); err != nil {
				fmt.Printf("Task %d was not allowed: %v\n", taskIndex, err)
				return
			}
			err := pool.Submit(ctx, func(ctx context.Context) error {
				time.Sleep(500 * time.Millisecond)
				fmt.Printf("Task %d completed\n", taskIndex)
				return nil
			})
			if err != nil {
				fmt.Printf("Submit error for task %d: %v\n", taskIndex, err)
			}
		}()
	}

	// Wait for all task submissions to complete
	wg.Wait()

	// Wait for all tasks to complete or fail
	if err := pool.Wait(); err != nil {
		fmt.Println("Pool completed with errors:", err)
	} else {
		fmt.Println("All tasks completed successfully.")
	}
}
