package gomoresync

import (
	"context"
	"errors"
	"sync"
)

// SubmitBehavior defines how Submit is called when the task queue is full.
type SubmitBehavior int

const (
	SubmitBlock SubmitBehavior = iota
	SubmitError
)

// ErrorAggregator defines how errors are aggregated in the pool.
type ErrorAggregator interface {
	Add(error)
	Error() error
}

// FirstErrorAggregator captures only the first non-nil error.
type FirstErrorAggregator struct {
	err   error
	errMu sync.Mutex
}

func (a *FirstErrorAggregator) Add(err error) {
	if err == nil {
		return
	}
	a.errMu.Lock()
	defer a.errMu.Unlock()
	if a.err == nil {
		a.err = err
	}
}

func (a *FirstErrorAggregator) Error() error {
	a.errMu.Lock()
	defer a.errMu.Unlock()
	return a.err
}

// AllErrorAggregator collects all non-nil errors.
type AllErrorAggregator struct {
	errors []error
	errMu  sync.Mutex
}

func (a *AllErrorAggregator) Add(err error) {
	if err == nil {
		return
	}
	a.errMu.Lock()
	defer a.errMu.Unlock()
	a.errors = append(a.errors, err)
}

func (a *AllErrorAggregator) Error() error {
	a.errMu.Lock()
	defer a.errMu.Unlock()
	if len(a.errors) == 0 {
		return nil
	}
	return &AggregateError{Errors: append([]error(nil), a.errors...)}
}

// AggregateError represents multiple errors.
type AggregateError struct {
	Errors []error
}

func (e *AggregateError) Error() string {
	errMsg := "Multiple errors occurred:"
	for _, err := range e.Errors {
		errMsg += "\n- " + err.Error()
	}
	return errMsg
}

// Observer interface for hooking into the task lifecycle.
type Observer interface {
	OnTaskStart(taskID int)
	OnTaskCompleted(taskID int, err error)
}

type nopObserver struct{}

func (nopObserver) OnTaskStart(int)            {}
func (nopObserver) OnTaskCompleted(int, error) {}

// PoolConfig holds the configuration for the Pool.
type PoolConfig struct {
	MaxWorkers      int
	BufferSize      int
	SubmitBehavior  SubmitBehavior
	ErrorAggregator ErrorAggregator
	Observer        Observer
}

// PoolOption defines a function type for configuring the Pool.
type PoolOption func(*PoolConfig)

// WithMaxWorkers sets the maximum number of worker goroutines.
func WithMaxWorkers(n int) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.MaxWorkers = n
	}
}

// WithBufferSize sets the channel buffer size for tasks.
func WithBufferSize(size int) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.BufferSize = size
	}
}

// WithSubmitBehavior sets the submit behavior (block or error) when queue is full.
func WithSubmitBehavior(behavior SubmitBehavior) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.SubmitBehavior = behavior
	}
}

// WithErrorAggregator sets a custom error aggregator.
func WithErrorAggregator(aggregator ErrorAggregator) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.ErrorAggregator = aggregator
	}
}

// WithObserver sets a custom observer for task lifecycle callbacks.
func WithObserver(obs Observer) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.Observer = obs
	}
}

// Pool is a concurrency construct that manages a pool of workers to process tasks concurrently.
type Pool struct {
	maxWorkers      int
	tasksChan       chan taskWrapper
	wg              sync.WaitGroup
	mu              sync.Mutex
	closed          bool
	submitBehavior  SubmitBehavior
	errorAggregator ErrorAggregator
	observer        Observer

	taskIDCounter int
}

type taskWrapper struct {
	task   func(ctx context.Context) error
	ctx    context.Context
	taskID int
}

// NewPool creates a new Pool with the given options.
func NewPool(options ...PoolOption) (*Pool, error) {
	cfg := &PoolConfig{
		MaxWorkers:      10,
		BufferSize:      20,
		SubmitBehavior:  SubmitBlock,
		ErrorAggregator: &FirstErrorAggregator{},
		Observer:        nopObserver{},
	}

	for _, opt := range options {
		opt(cfg)
	}

	if cfg.MaxWorkers <= 0 {
		return nil, errors.New("MaxWorkers must be > 0")
	}
	if cfg.BufferSize < 0 {
		return nil, errors.New("BufferSize must be >= 0")
	}
	if cfg.ErrorAggregator == nil {
		return nil, errors.New("ErrorAggregator must not be nil")
	}
	if cfg.Observer == nil {
		cfg.Observer = nopObserver{}
	}

	p := &Pool{
		maxWorkers:      cfg.MaxWorkers,
		submitBehavior:  cfg.SubmitBehavior,
		errorAggregator: cfg.ErrorAggregator,
		observer:        cfg.Observer,
		tasksChan:       make(chan taskWrapper, cfg.BufferSize),
	}

	p.startWorkers()
	return p, nil
}

// startWorkers initializes the pool's goroutines.
func (p *Pool) startWorkers() {
	for i := 0; i < p.maxWorkers; i++ {
		p.wg.Add(1)
		go p.workerLoop()
	}
}

// workerLoop represents a single worker's lifecycle.
func (p *Pool) workerLoop() {
	defer p.wg.Done()
	for tw := range p.tasksChan {
		p.observer.OnTaskStart(tw.taskID)

		// If the context is already done, skip the task
		select {
		case <-tw.ctx.Done():
			p.observer.OnTaskCompleted(tw.taskID, tw.ctx.Err())
			p.captureError(tw.ctx.Err())
			continue
		default:
		}

		err := tw.task(tw.ctx)
		p.observer.OnTaskCompleted(tw.taskID, err)
		if err != nil {
			p.captureError(err)
		}
	}
}

// captureError records errors using the ErrorAggregator.
func (p *Pool) captureError(err error) {
	p.errorAggregator.Add(err)
}

// Shutdown attempts to finish all existing tasks and close the pool.
// If the provided context is canceled before all tasks are complete,
// Shutdown returns a context error. Otherwise, it returns any errors from tasks.
func (p *Pool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		err := p.errorAggregator.Error()
		p.mu.Unlock()
		return err
	}
	p.closed = true
	close(p.tasksChan)
	p.mu.Unlock()

	doneCh := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(doneCh)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-doneCh:
		return p.errorAggregator.Error()
	}
}

// Submit schedules a task for execution in the worker pool.
// The task is a function that receives a context, so it can respect cancellation/timeouts.
func (p *Pool) Submit(ctx context.Context, task func(ctx context.Context) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return errors.New("cannot submit task to a closed worker pool")
	}

	// Increment a local task ID for debugging/observer usage
	p.taskIDCounter++
	id := p.taskIDCounter

	// Behavior control
	switch p.submitBehavior {
	case SubmitBlock:
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p.tasksChan <- taskWrapper{task: task, ctx: ctx, taskID: id}:
			return nil
		}
	case SubmitError:
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p.tasksChan <- taskWrapper{task: task, ctx: ctx, taskID: id}:
			return nil
		default:
			return errors.New("task queue is full")
		}
	}
	return nil
}

func SubmitWithRateLimit(ctx context.Context, pool *Pool, limiter RateLimiter, task func(context.Context) error) error {
	if err := limiter.Wait(ctx); err != nil {
		return err
	}
	return pool.Submit(ctx, task)
}

// Wait blocks until all submitted tasks have finished, then returns any aggregated error.
func (p *Pool) Wait() error {
	p.closeTasksChan()
	p.wg.Wait()
	return p.errorAggregator.Error()
}

// closeTasksChan ensures the task channel is closed only once.
func (p *Pool) closeTasksChan() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.tasksChan)
	}
}

// Resize allows dynamically changing the number of workers at runtime.
// If newMaxWorkers < current number of workers, the extra workers finish their current task and exit.
func (p *Pool) Resize(newMaxWorkers int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if newMaxWorkers <= 0 {
		return errors.New("newMaxWorkers must be > 0")
	}
	if p.closed {
		return errors.New("cannot resize a closed pool")
	}

	current := p.maxWorkers
	p.maxWorkers = newMaxWorkers

	if newMaxWorkers > current {
		// Start additional workers
		diff := newMaxWorkers - current
		for i := 0; i < diff; i++ {
			p.wg.Add(1)
			go p.workerLoop()
		}
	}
	// Let workers exit naturally by feeding no additional tasks.
	// The newly 'excess' workers will eventually exit once tasksChan is drained,
	// or once the pool is shut down. There’s no immediate forced kill here.
	return nil
}
