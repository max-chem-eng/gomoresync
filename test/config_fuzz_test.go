package test

import (
	"testing"

	"github.com/max-chem-eng/gomoresync"
)

func FuzzPoolConfig(f *testing.F) {
	f.Add(10, 20) // seed with some values
	f.Fuzz(func(t *testing.T, maxWorkers int, bufferSize int) {
		// Fuzzing the config input values
		if maxWorkers <= 0 {
			maxWorkers = 1
		}
		if bufferSize < 0 {
			bufferSize = 0
		}
		_, err := gomoresync.NewPool(gomoresync.WithMaxWorkers(maxWorkers), gomoresync.WithBufferSize(bufferSize))
		// Just ensure no panic occurs, error can be expected with invalid configs
		if err != nil && maxWorkers > 0 && bufferSize >= 0 {
			t.Errorf("unexpected error with valid config: %v", err)
		}
	})
}
