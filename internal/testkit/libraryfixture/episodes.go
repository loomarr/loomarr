package libraryfixture

import (
	"context"
	"sync"

	"github.com/mantonx/loomarr/internal/schedule"
)

// Episodes is the shared no-network adapter for library episode resolution.
type Episodes struct {
	mu      sync.Mutex
	Results map[string][]schedule.ResolvedProgram
	Err     error
	calls   []string
}

// NewEpisodes builds an episode adapter keyed by library show id.
func NewEpisodes(results map[string][]schedule.ResolvedProgram) *Episodes {
	return &Episodes{Results: results}
}

// Resolve returns the configured episodes without opening a socket.
func (e *Episodes) Resolve(_ context.Context, showItemID string) ([]schedule.ResolvedProgram, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, showItemID)
	return append([]schedule.ResolvedProgram(nil), e.Results[showItemID]...), e.Err
}

// Calls returns the library show ids observed by this adapter.
func (e *Episodes) Calls() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.calls...)
}
