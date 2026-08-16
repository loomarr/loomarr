// Package catalogfixture provides shared no-network adapters for catalog tests.
package catalogfixture

import (
	"context"
	"sync"

	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/provision"
)

// Corpus is a deterministic search/discovery corpus.
type Corpus struct {
	Candidates []catalog.Candidate
}

func (c *Corpus) Search(context.Context, string, int) ([]catalog.Candidate, error) {
	return append([]catalog.Candidate(nil), c.Candidates...), nil
}

func (c *Corpus) Discover(context.Context, provision.MediaType, []string, int, int, int) ([]catalog.Candidate, error) {
	return append([]catalog.Candidate(nil), c.Candidates...), nil
}

// Presence is a deterministic library-ownership adapter.
type Presence struct {
	mu    sync.Mutex
	Hits  map[int]catalog.Presence
	calls []int
}

func (p *Presence) Present(
	_ context.Context,
	_ provision.MediaType,
	tmdbID, _ int,
) (catalog.Presence, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, tmdbID)
	hit, ok := p.Hits[tmdbID]
	return hit, ok, nil
}

func (p *Presence) Calls() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.calls...)
}
