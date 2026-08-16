package libraryfixture

import (
	"context"
	"sync"

	"github.com/mantonx/loomarr/internal/library"
)

// LookupResult is one deterministic media-library presence answer.
type LookupResult struct {
	ItemID  string
	Present bool
	Err     error
}

// Lookup is the shared no-network adapter for library presence consumers.
type Lookup struct {
	mu      sync.Mutex
	Results map[string]LookupResult
	calls   []string
}

// NewLookup builds a presence adapter keyed by provider id.
func NewLookup(results map[string]LookupResult) *Lookup {
	return &Lookup{Results: results}
}

// Lookup implements reconcile.LibraryLookup without opening a socket.
func (l *Lookup) Lookup(
	_ context.Context,
	_ library.ProviderKind,
	providerID string,
	_ library.MediaType,
) (string, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, providerID)
	result := l.Results[providerID]
	return result.ItemID, result.Present, result.Err
}

// Calls returns the provider ids observed by this adapter.
func (l *Lookup) Calls() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}
