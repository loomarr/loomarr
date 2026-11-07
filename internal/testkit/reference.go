package testkit

import (
	"context"
	"errors"
	"sync"

	"github.com/loomarr/loomarr/internal/proposals/webreference"
)

// ReferenceResolver is the shared no-network double for reference-backed Intent
// tests. Calls returns copied lookups so assertions do not race background jobs.
type ReferenceResolver struct {
	mu                sync.Mutex
	Evidence          webreference.Evidence
	Err               error
	ByURL             map[string]webreference.Evidence
	ByLabel           map[string]webreference.Evidence
	DiscoveryEvidence webreference.Evidence
	DiscoveryErr      error
	discoveries       []string
	lookups           []webreference.Lookup
}

func (r *ReferenceResolver) Lookup(_ context.Context, lookup webreference.Lookup) (webreference.Evidence, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lookups = append(r.lookups, lookup)
	if r.ByURL != nil {
		evidence, ok := r.ByURL[lookup.URL]
		if !ok {
			return webreference.Evidence{}, errors.New("testkit: reference is not frozen")
		}
		return evidence, r.Err
	}
	return r.Evidence, r.Err
}

func (r *ReferenceResolver) Calls() []webreference.Lookup {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]webreference.Lookup(nil), r.lookups...)
}

func (r *ReferenceResolver) Discover(_ context.Context, label string) (webreference.Evidence, error) {
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
