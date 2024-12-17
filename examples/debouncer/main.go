package main

import (
	"fmt"
	"time"

	"github.com/max-chem-eng/gomoresync"
)

func main() {
	// Debounce function that fires 1 second after the last call
	debounced := gomoresync.Debounce(func() {
		fmt.Println("Debounced function executed!")
	}, 1*time.Second)

	for i := 0; i < 5; i++ {
		debounced()
		fmt.Printf("Call %d\n", i)
		time.Sleep(300 * time.Millisecond)
	}
	// If we wait 1 second after the last call, the function will run once
	time.Sleep(2 * time.Second)
	fmt.Println("Debouncer example completed.")
}
