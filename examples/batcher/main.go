package main

import (
	"fmt"
	"log"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

// Run this example to see how the batcher works.
func main() {
	// Flush function simply prints the batched items
	flushFn := func(items []interface{}) {
		fmt.Printf("Flushing batch of %d items: %v\n", len(items), items)
	}

	batcher, err := gomoresync.NewBatcher(
		gomoresync.BatcherConfig{
			MaxBatchSize:  5,
			BatchInterval: 2 * time.Second,
		},
		flushFn,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer batcher.Close()

	// Add items to the batcher
	for i := 0; i < 12; i++ {
		batcher.Add(i)
		fmt.Printf("Added item %d\n", i)
		time.Sleep(500 * time.Millisecond)
	}

	// Wait a bit for final flush
	time.Sleep(3 * time.Second)
	fmt.Println("Batcher example completed.")
}
