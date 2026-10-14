package testkit

import (
	"context"
	"sync"
)

// SnapshotLoader is a reusable, in-memory source for consumers that load one
// coherent runtime snapshot. clone keeps mutable values owned by the fixture.
type SnapshotLoader[T any] struct {
	mu      sync.Mutex
	value   T
	err     error
	reads   int
	clone   func(T) T
	loaded  chan struct{}
	release chan struct{}
}

// NewSnapshotLoader builds a snapshot source with the supplied initial value.
func NewSnapshotLoader[T any](value T, clone func(T) T) *SnapshotLoader[T] {
	return &SnapshotLoader[T]{value: clone(value), clone: clone}
}

// LoadSnapshot returns one cloned generation. BlockNextRead can pause exactly
// one call after it captures that generation to exercise publication ordering.
func (l *SnapshotLoader[T]) LoadSnapshot(ctx context.Context) (T, error) {
	l.mu.Lock()
	l.reads++
	value, err := l.clone(l.value), l.err
	loaded, release := l.loaded, l.release
	l.loaded, l.release = nil, nil
	l.mu.Unlock()

	if loaded != nil {
		close(loaded)
		select {
		case <-release:
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}
	return value, err
}

// Set replaces the generation or error returned by future reads.
func (l *SnapshotLoader[T]) Set(value T, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.value = l.clone(value)
	l.err = err
}

// Value returns a clone of the currently configured generation.
func (l *SnapshotLoader[T]) Value() T {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.clone(l.value)
}

// Reads reports how many snapshot loads the fixture observed.
func (l *SnapshotLoader[T]) Reads() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reads
}

// BlockNextRead pauses the next load after it captures its generation. The
// returned channel closes at that point; release unblocks the load once.
func (l *SnapshotLoader[T]) BlockNextRead() (<-chan struct{}, func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.loaded != nil {
		panic("testkit: snapshot read already blocked")
	}
	l.loaded = make(chan struct{})
	l.release = make(chan struct{})
	loaded, release := l.loaded, l.release
	var once sync.Once
	return loaded, func() { once.Do(func() { close(release) }) }
}

// ApplyFunc adapts a function to interfaces whose write method is named Apply.
type ApplyFunc[T any] func(context.Context, T) error

// Apply invokes f.
func (f ApplyFunc[T]) Apply(ctx context.Context, value T) error { return f(ctx, value) }
