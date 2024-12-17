package gomoresync

import (
	"errors"
	"sync"
	"time"
)

// BatcherConfig configures the Batcher.
type BatcherConfig struct {
	MaxBatchSize  int           // trigger flush when batch size reaches this limit
	BatchInterval time.Duration // flush after this duration if there's at least 1 item
}

// Batcher aggregates items until MaxBatchSize or BatchInterval is reached,
// then calls a user-specified flush function in a separate goroutine.
type Batcher struct {
	cfg   BatcherConfig
	mu    sync.Mutex
	queue []interface{}

	flushFn func([]interface{}) // user-defined callback

	ticker *time.Ticker
	stopCh chan struct{}
	closed bool
}

// NewBatcher initializes a new Batcher.
func NewBatcher(cfg BatcherConfig, flushFn func([]interface{})) (*Batcher, error) {
	if cfg.MaxBatchSize <= 0 {
		return nil, errors.New("MaxBatchSize must be > 0")
	}
	if cfg.BatchInterval <= 0 {
		return nil, errors.New("BatchInterval must be > 0")
	}
	if flushFn == nil {
		return nil, errors.New("flush function must not be nil")
	}

	b := &Batcher{
		cfg:     cfg,
		flushFn: flushFn,
		queue:   make([]interface{}, 0, cfg.MaxBatchSize),
		stopCh:  make(chan struct{}),
	}
	b.ticker = time.NewTicker(cfg.BatchInterval)

	// Start the flushing goroutine
	go b.run()
	return b, nil
}

// Add adds an item to the batch. If the batch becomes full, flush happens immediately.
func (b *Batcher) Add(item interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.queue = append(b.queue, item)
	if len(b.queue) >= b.cfg.MaxBatchSize {
		b.flushLocked()
	}
}

// run checks every BatchInterval to see if the batch needs flushing.
func (b *Batcher) run() {
	for {
		select {
		case <-b.stopCh:
			return
		case <-b.ticker.C:
			b.mu.Lock()
			if b.closed {
				b.mu.Unlock()
				return
			}
			if len(b.queue) > 0 {
				b.flushLocked()
			}
			b.mu.Unlock()
		}
	}
}

// flushLocked flushes the current queue in a goroutine so that flushing is non-blocking.
func (b *Batcher) flushLocked() {
	toFlush := append([]interface{}(nil), b.queue...)
	b.queue = b.queue[:0] // reset
	go b.flushFn(toFlush)
}

// Close stops the batcher, flushes any remaining items, and marks it as closed.
func (b *Batcher) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	b.ticker.Stop()
	close(b.stopCh)

	// Flush leftover items
	if len(b.queue) > 0 {
		b.flushLocked()
	}
}
