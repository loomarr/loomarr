package testkit

import (
	"context"
	"sync"
)

// TunerRescanner is the shared in-memory double for operation-specific media-server channel-list
// refreshes. Err injects a best-effort poke failure.
type TunerRescanner struct {
	mu    sync.Mutex
	Err   error
	calls int
}

func (r *TunerRescanner) RescanTuner(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.Err
}

// Calls returns the number of tuner re-scans requested.
func (r *TunerRescanner) Calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}
