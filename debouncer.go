package gomoresync

import (
	"sync"
	"time"
)

// Debounce returns a function that delays the execution of fn until `delay` has passed
// since the last call to the returned function.
func Debounce(fn func(), delay time.Duration) func() {
	var mu sync.Mutex
	var timer *time.Timer

	return func() {
		mu.Lock()
		defer mu.Unlock()

		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(delay, fn)
	}
}
