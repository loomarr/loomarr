package testkit

import (
	"context"
	"errors"
	"sync"

	"github.com/loomarr/loomarr/internal/reference"
)

// ReferenceResolver is the shared no-network double for reference-backed Intent
// tests. Calls returns copied lookups so assertions do not race background jobs.
type ReferenceResolver struct {
	mu                sync.Mutex
	Evidence          reference.Evidence
	Err               error
	ByURL             map[string]reference.Evidence
	ByLabel           map[string]reference.Evidence
	DiscoveryEvidence reference.Evidence
	DiscoveryErr      error
	discoveries       []string
	lookups           []reference.Lookup
}

func (r *ReferenceResolver) Lookup(_ context.Context, lookup reference.Lookup) (reference.Evidence, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lookups = append(r.lookups, lookup)
	if r.ByURL != nil {
		evidence, ok := r.ByURL[lookup.URL]
		if !ok {
			return reference.Evidence{}, errors.New("testkit: reference is not frozen")
		}
		return evidence, r.Err
	}
	return r.Evidence, r.Err
}

func (r *ReferenceResolver) Calls() []reference.Lookup {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]reference.Lookup(nil), r.lookups...)
}

func (r *ReferenceResolver) Discover(_ context.Context, label string) (reference.Evidence, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.discoveries = append(r.discoveries, label)
	if r.ByLabel != nil {
		return r.ByLabel[label], r.DiscoveryErr
	}
	return r.DiscoveryEvidence, r.DiscoveryErr
}

func (r *ReferenceResolver) Discoveries() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.discoveries...)
}
