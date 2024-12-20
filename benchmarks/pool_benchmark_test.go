package benchmarks

import (
	"context"
	"testing"

	"github.com/max-chem-eng/gomoresync"
)

func BenchmarkPool_Submit(b *testing.B) {
	pool, _ := gomoresync.NewPool(gomoresync.WithMaxWorkers(10), gomoresync.WithBufferSize(100))
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Submit(ctx, func(ctx context.Context) error {
			// minimal task
			return nil
		})
	}
	b.StopTimer()
	pool.Wait()
}
