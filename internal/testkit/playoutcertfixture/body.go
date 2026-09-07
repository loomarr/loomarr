package playoutcertfixture

import (
	"io"
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
