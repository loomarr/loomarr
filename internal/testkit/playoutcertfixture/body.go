package playoutcertfixture

import (
	"io"
	"sync"
	"sync/atomic"
)

// CountingReadCloser records closure of a wrapped response body for lifecycle
// assertions while preserving the underlying transport's read behavior.
type CountingReadCloser struct {
	io.ReadCloser
	Closed *atomic.Int32
}

func (r *CountingReadCloser) Close() error {
	r.Closed.Add(1)
	return r.ReadCloser.Close()
}

// GatedTerminalBody passes a known-good prefix from its source, then waits for
// Gate before returning Terminal. Close releases a pending Read so tests can
// assert that cancellation joins a streaming reader.
type GatedTerminalBody struct {
	Source   io.ReadCloser
	Gate     <-chan struct{}
	Prefix   int
	Terminal error
	Closed   *atomic.Int32
	// ReadStarted is notified immediately before a terminal read waits on Gate.
	ReadStarted chan<- struct{}

	mu        sync.Mutex
	remaining int
	closed    chan struct{}
	once      sync.Once
}

func NewGatedTerminalBody(source io.ReadCloser, gate <-chan struct{}, prefix int, terminal error, closed *atomic.Int32) *GatedTerminalBody {
	return &GatedTerminalBody{Source: source, Gate: gate, Prefix: prefix, Terminal: terminal, Closed: closed, remaining: prefix, closed: make(chan struct{})}
}

func (b *GatedTerminalBody) Read(buffer []byte) (int, error) {
	b.mu.Lock()
	remaining := b.remaining
	b.mu.Unlock()
	if remaining > 0 {
		if len(buffer) > remaining {
			buffer = buffer[:remaining]
		}
		n, err := b.Source.Read(buffer)
		if n > 0 {
			b.mu.Lock()
			b.remaining -= n
			b.mu.Unlock()
		}
		return n, err
	}
	select {
	case b.ReadStarted <- struct{}{}:
	default:
	}
	select {
	case <-b.Gate:
		return 0, b.Terminal
	case <-b.closed:
		return 0, io.ErrClosedPipe
	}
}

func (b *GatedTerminalBody) Close() error {
	b.once.Do(func() {
		close(b.closed)
		if b.Closed != nil {
			b.Closed.Add(1)
		}
	})
	return b.Source.Close()
}
