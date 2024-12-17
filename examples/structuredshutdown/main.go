package main

import (
	"context"
	"fmt"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	group := gomoresync.NewGoroutineGroup(ctx)

	// Start multiple goroutines
	for i := 0; i < 5; i++ {
		i := i
		group.Go(func(ctx context.Context) error {
			ticker := time.NewTicker(300 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					fmt.Printf("Goroutine %d received shutdown signal\n", i)
					return ctx.Err()
				case <-ticker.C:
					fmt.Printf("Goroutine %d is working...\n", i)
				}
			}
		})
	}

	// Let them run briefly
	time.Sleep(1 * time.Second)
	fmt.Println("Canceling the group context to stop all goroutines...")
	group.Cancel()

	// Wait for completion
	errs := group.Wait()
	if errs != nil {
		fmt.Println("Errors from goroutines:", errs)
	} else {
		fmt.Println("All goroutines finished without error.")
	}
}
