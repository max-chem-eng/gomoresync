package gomoresync

import (
	"context"
	"errors"
	"sync"
)

// SubmitBehavior defines what happens when Submit is called and the task queue is full.
type SubmitBehavior int

// enum for SubmitBehavior
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

type Observer interface {
	OnTaskStart()
	OnTaskCompleted(err error)
}

type nopObserver struct{}

func (nopObserver) OnTaskStart()            {}
func (nopObserver) OnTaskCompleted(e error) {}

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
	return &AggregateError{Errors: a.errors}
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

func WithMaxWorkers(n int) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.MaxWorkers = n
	}
}

func WithBufferSize(size int) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.BufferSize = size
	}
}

func WithSubmitBehavior(behavior SubmitBehavior) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.SubmitBehavior = behavior
	}
}

func WithErrorAggregator(aggregator ErrorAggregator) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.ErrorAggregator = aggregator
	}
}

func WithObserver(obs Observer) PoolOption {
	return func(cfg *PoolConfig) {
		cfg.Observer = obs
	}
}

// Pool is a concurrency construct that manages a pool of workers to process tasks concurrently.
// Tasks are functions that return an error. The pool can be configured with various behaviors:
// - maxWorkers: how many workers run in parallel.
// - bufferSize: size of the internal queue of tasks.
// - submitBehavior: whether submitting a task blocks or returns an error if the queue is full.
// - errorAggregator: how to handle multiple task errors.
// - observer: hooks for monitoring task lifecycle.
//
// Example usage:
//
//	p, err := NewPool(WithMaxWorkers(5), WithBufferSize(10))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	for i := 0; i < 100; i++ {
//	    p.Submit(ctx, func() error {
//	        // do something
//	        return nil
//	    })
//	}
//
//	if err := p.Wait(); err != nil {
//	    // handle errors
//	}
type Pool struct {
	maxWorkers      int
	tasksChan       chan func() error
	wg              sync.WaitGroup
	mu              sync.Mutex
	closed          bool
	submitBehavior  SubmitBehavior
	errorAggregator ErrorAggregator
	observer        Observer
}

// NewPool creates a new Pool with the given options.
func NewPool(options ...PoolOption) (*Pool, error) {
	cfg := &PoolConfig{
		MaxWorkers:      10,
		BufferSize:      20,
		SubmitBehavior:  SubmitBlock,
		ErrorAggregator: &FirstErrorAggregator{},
	}
	for _, opt := range options {
		opt(cfg)
	}

	if cfg.Observer == nil {
		cfg.Observer = nopObserver{}
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

	p := &Pool{
		maxWorkers:      cfg.MaxWorkers,
		tasksChan:       make(chan func() error, cfg.BufferSize),
		submitBehavior:  cfg.SubmitBehavior,
		errorAggregator: cfg.ErrorAggregator,
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
	for task := range p.tasksChan {
		p.observer.OnTaskStart()
		err := task()
		p.observer.OnTaskCompleted(err)
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
	// If already closed, just return whatever errors we have.
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
		// Context was canceled before tasks finished
		return ctx.Err()
	case <-doneCh:
		return p.errorAggregator.Error()
	}
}

func (p *Pool) Submit(ctx context.Context, task func() error) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return errors.New("cannot submit task to a closed worker pool")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	switch p.submitBehavior {
	case SubmitBlock:
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p.tasksChan <- task:
			return nil
		}
	case SubmitError:
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p.tasksChan <- task:
			return nil
		default:
			return errors.New("task queue is full")
		}
	}

	return nil
}

// Wait blocks until all submitted tasks have finished.
func (p *Pool) Wait() error {
	p.closeTasksChan()
	p.wg.Wait()
	return p.errorAggregator.Error()
}

// closeTasksChan ensures the task channel is closed only once.
func (p *Pool) closeTasksChan() {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.tasksChan)
	}
	p.mu.Unlock()
}
