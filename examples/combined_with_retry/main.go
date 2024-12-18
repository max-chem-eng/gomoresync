package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func main() {
	rootCtx := context.Background()

	// Prepare a RetryConfig with exponential backoff and jitter
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

	// Set up an HTTP handler that uses the retry logic
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a context for the operation
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		// Define the operation to retry
		operation := func() error {
			// Simulate a failing operation
			return errors.New("operation failed")
		}

		// Use the Retry function
		err := gomoresync.Retry(ctx, operation, retryCfg)
		if err != nil {
			http.Error(w, "Operation failed after retries", http.StatusInternalServerError)
			return
		}

		w.Write([]byte("Operation succeeded"))
	})

	// Start the HTTP server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	log.Println("Server started on :8080")

	// Wait for shutdown signal
	<-rootCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}

	log.Println("Server exited")
}
