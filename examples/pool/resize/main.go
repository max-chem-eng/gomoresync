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
		gomoresync.WithMaxWorkers(2),
		gomoresync.WithBufferSize(2),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		i := i
		_ = pool.Submit(ctx, func(ctx context.Context) error {
			time.Sleep(300 * time.Millisecond)
			fmt.Printf("Task %d completed on initial worker count.\n", i)
			return nil
		})
	}

	// Wait a bit then resize
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Resizing pool to 4 workers...")
	if err := pool.Resize(4); err != nil {
		log.Println("resize error:", err)
	}

	// Add more tasks
	for i := 5; i < 10; i++ {
		i := i
		_ = pool.Submit(ctx, func(ctx context.Context) error {
			time.Sleep(300 * time.Millisecond)
			fmt.Printf("Task %d completed after resize.\n", i)
			return nil
		})
	}

	if err := pool.Wait(); err != nil {
		fmt.Println("Pool completed with errors:", err)
	} else {
		fmt.Println("All tasks completed successfully after resize.")
	}
}
