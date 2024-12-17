package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Example operation that fails the first few times, then succeeds
	attempt := 0
	operation := func() error {
		attempt++
		if attempt < 3 {
			return errors.New("transient error")
		}
		return nil
	}

	cfg := gomoresync.RetryConfig{
		MaxRetries:    5,
		LastErrorOnly: false,
		Backoff: gomoresync.JitterBackoff{
			Delegate: gomoresync.ExponentialBackoff{
				BaseDelay: 500 * time.Millisecond,
				Factor:    2,
				MaxDelay:  5 * time.Second,
			},
			JitterFactor: 0.3,
		},
	}

	err := gomoresync.Retry(ctx, operation, cfg)
	if err != nil {
		fmt.Println("Operation failed after retries:", err)
	} else {
		fmt.Println("Operation succeeded after retries!")
	}
}
