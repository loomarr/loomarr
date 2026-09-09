package playout

import (
	"context"
	"io"
	"sync"
)

const (
	transportReadSize = 64 << 10
	viewerBufferBytes = 8 * transportReadSize
)

// Stream is an ordered raw transport with cancellable reads. Next returns an owned
// chunk or an error; io.EOF follows the last accepted byte. Release the associated
// Presentation when finished, including after cancellation or EOF.
type Stream interface {
	Next(context.Context) ([]byte, error)
}

// streamViewer owns one fixed byte ring, allocated on the first offer. It has no
// forwarding goroutine or secondary queue; pipe fragmentation cannot consume its
// capacity through slice descriptors. One consumer calls Next.
type streamViewer struct {
	mu         sync.Mutex
	buffer     []byte
	head, used int
	closed     bool
	notify     chan struct{}
}

func newStreamViewer() *streamViewer {
	return &streamViewer{notify: make(chan struct{}, 1)}
}

func (v *streamViewer) offer(chunk []byte) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || len(chunk) > viewerBufferBytes-v.used {
		return false
	}
	if len(chunk) == 0 {
		return true
	}
	if v.buffer == nil {
		v.buffer = make([]byte, viewerBufferBytes)
	}
	tail := (v.head + v.used) % viewerBufferBytes
	first := copy(v.buffer[tail:], chunk)
	copy(v.buffer, chunk[first:])
	v.used += len(chunk)
	v.signal()
	return true
}

func (v *streamViewer) Next(ctx context.Context) ([]byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v.mu.Lock()
		if v.used > 0 {
			chunk := make([]byte, min(v.used, transportReadSize))
			first := copy(chunk, v.buffer[v.head:])
			copy(chunk[first:], v.buffer[:len(chunk)-first])
			v.head = (v.head + len(chunk)) % viewerBufferBytes
			v.used -= len(chunk)
			if v.used == 0 && v.closed {
				v.buffer = nil
			}
			v.mu.Unlock()
			return chunk, nil
		}
		closed := v.closed
		v.mu.Unlock()
		if closed {
			return nil, io.EOF
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-v.notify:
		}
	}
}

// close preserves accepted bytes at producer EOF. It does not wait for the reader.
func (v *streamViewer) close() {
	v.mu.Lock()
	v.closed = true
	if v.used == 0 {
		v.buffer = nil
	}
	v.signal()
	v.mu.Unlock()
}

// discard is caller release: stop reads even if Session already removed the viewer.
func (v *streamViewer) discard() {
	v.mu.Lock()
	v.closed = true
	v.buffer = nil
	v.used = 0
	v.signal()
	v.mu.Unlock()
}

func (v *streamViewer) signal() {
	select {
	case v.notify <- struct{}{}:
	default:
	}
}
