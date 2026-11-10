package testkit

import (
	"context"
	"sync"

	"github.com/loomarr/loomarr/internal/library/provision"
)

// Requester is the shared Requester test double (AGENTS.md). It records calls and
// lets a test control whether Request/Cancel succeed.
type Requester struct {
	mu sync.Mutex
	// RequestErr, if set, is returned by every Request (simulates a down Seerr).
	RequestErr error
	// CancelErr, if set, is returned by every Cancel.
	CancelErr error

	Requested []provision.Title
	Cancelled []provision.Title
}

// Request records the call. Like Seerr and the arrs, it refuses a title with no usable
// provider id before anything else.
func (r *Requester) Request(_ context.Context, t provision.Title) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Requested = append(r.Requested, t)
	if _, err := t.Key(); err != nil {
		return err
	}
	return r.RequestErr
}

func (r *Requester) Cancel(_ context.Context, t provision.Title) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Cancelled = append(r.Cancelled, t)
	return r.CancelErr
}

// RequestCount / CancelCount are race-safe accessors for assertions.
func (r *Requester) RequestCount() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.Requested) }
func (r *Requester) CancelCount() int  { r.mu.Lock(); defer r.mu.Unlock(); return len(r.Cancelled) }
