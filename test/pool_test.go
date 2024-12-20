package test

import (
	"context"
	"testing"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func TestPool_BasicFunctionality(t *testing.T) {
	pool, err := gomoresync.NewPool(
		gomoresync.WithMaxWorkers(2),
		gomoresync.WithBufferSize(5),
		gomoresync.WithErrorAggregator(&gomoresync.AllErrorAggregator{}),
	)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	taskCount := 10
	doneCh := make(chan struct{})

	go func() {
		for i := 0; i < taskCount; i++ {
			err := pool.Submit(context.Background(), func(ctx context.Context) error {
				// Each task sleeps briefly to simulate work
				time.Sleep(100 * time.Millisecond)
				return nil
			})
			if err != nil {
				t.Errorf("unexpected submit error for task %d: %v", i, err)
			}
		}
		close(doneCh)
	}()

	select {
	case <-doneCh:
		// All tasks submitted successfully
	case <-time.After(5 * time.Second):
		t.Fatal("timed out submitting tasks - tasks took too long")
	}

	// Wait for all tasks to complete
	if werr := pool.Wait(); werr != nil {
		t.Errorf("pool returned error after Wait(): %v", werr)
	} else {
		t.Log("All tasks completed successfully with no errors.")
	}
}

func TestPool_CloseEarly(t *testing.T) {
	pool, err := gomoresync.NewPool(
		gomoresync.WithMaxWorkers(2),
		gomoresync.WithBufferSize(5),
	)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	// Submit a few long-running tasks
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		i := i
		err := pool.Submit(ctx, func(ctx context.Context) error {
			// Simulate a slow task
			time.Sleep(500 * time.Millisecond)
			return nil
		})
		if err != nil {
			t.Errorf("unexpected submit error for task %d: %v", i, err)
		}
	}

	// Attempt to shutdown before tasks finish
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer shutdownCancel()

	err = pool.Shutdown(shutdownCtx)
	if err != nil {
		// It's not necessarily an error if we get a context deadline error
		// due to the tasks not finishing in time. But we ensure no panic or unexpected error.
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Errorf("unexpected shutdown error: %v", err)
		}
	}

	// After shutting down, submitting a new task should fail
	err = pool.Submit(ctx, func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Errorf("expected error when submitting after shutdown, got none")
	}
}

// Additional tests:
// - TestPool_CloseEarly
// - TestPool_ResizeDown
// - TestPool_SubmitAfterClose
