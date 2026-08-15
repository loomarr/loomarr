package testkit

import (
	"context"
	"sync"
)

// BackendTransition is the shared in-memory double for the settings consequence seam.
type BackendTransition struct {
	mu      sync.Mutex
	Err     error
	targets []string
}

func (t *BackendTransition) Apply(_ context.Context, desired string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.targets = append(t.targets, desired)
	return t.Err
}

// Targets returns the desired backends passed to Apply in order.
func (t *BackendTransition) Targets() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.targets...)
}
