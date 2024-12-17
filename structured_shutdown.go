package gomoresync

import (
	"context"
	"sync"
)

// GoroutineGroup helps manage a set of goroutines with a shared context.
// When the context is canceled, all goroutines are signaled to stop.
type GoroutineGroup struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	errs   []error
}

// NewGoroutineGroup creates a new manager with a background context or user-supplied parent context.
func NewGoroutineGroup(parent context.Context) *GoroutineGroup {
	var ctx context.Context
	var cancel context.CancelFunc
	if parent == nil {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	return &GoroutineGroup{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Go spawns a new goroutine that runs fn until it returns or the context is done.
// Any error returned by fn is stored for retrieval later.
func (g *GoroutineGroup) Go(fn func(ctx context.Context) error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := fn(g.ctx); err != nil {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.errs = append(g.errs, err)
		}
	}()
}

// Wait blocks until all goroutines finish, then returns any collected errors.
func (g *GoroutineGroup) Wait() []error {
	g.wg.Wait()
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.errs) == 0 {
		return nil
	}
	return append([]error(nil), g.errs...)
}

// Cancel signals all goroutines to stop by canceling the context.
func (g *GoroutineGroup) Cancel() {
	g.cancel()
}

// Context returns the shared context used by the GoroutineGroup.
func (g *GoroutineGroup) Context() context.Context {
	return g.ctx
}
