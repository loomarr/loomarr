// Package playoutstreamfixture adapts controlled test chunks to cancellable transport reads.
package playoutstreamfixture

import (
	"context"
	"io"
)

// Channel is a test-owned stream whose producer controls bytes and EOF.
type Channel struct{ Chunks <-chan []byte }

func (s Channel) Next(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case chunk, ok := <-s.Chunks:
		if !ok {
			return nil, io.EOF
		}
		return chunk, nil
	}
}
