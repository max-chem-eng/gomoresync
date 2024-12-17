package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func main() {
	pool, err := gomoresync.NewPool(
		gomoresync.WithMaxWorkers(3),
		gomoresync.WithBufferSize(5),
		gomoresync.WithSubmitBehavior(gomoresync.SubmitBlock),
		gomoresync.WithErrorAggregator(&gomoresync.AllErrorAggregator{}),
	)
	if err != nil {
		log.Fatal("failed to create pool:", err)
	}

	// Submit a few tasks
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		i := i
		err := pool.Submit(ctx, func(ctx context.Context) error {
			time.Sleep(200 * time.Millisecond)
			fmt.Printf("Task %d processed\n", i)
			return nil
		})
		if err != nil {
			fmt.Printf("Submit error for task %d: %v\n", i, err)
		}
	}

	// Wait for tasks to complete
	if err := pool.Wait(); err != nil {
		fmt.Println("Errors occurred in pool:", err)
	} else {
		fmt.Println("All tasks completed successfully.")
	}
}
