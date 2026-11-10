// Package catalogfixture provides shared no-network adapters for catalog tests.
package catalogfixture

import (
	"context"
	"errors"
	"sync"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
)

// Corpus is a deterministic search/discovery corpus.
type Corpus struct {
	Candidates   []titlesearch.Candidate
	SearchFunc   func(context.Context, string, int) ([]titlesearch.Candidate, error)
	DiscoverFunc func(context.Context, titlesearch.DiscoveryQuery, int) ([]titlesearch.Candidate, error)
	mu           sync.Mutex
	searches     []SearchRequest
	discoveries  []DiscoveryRequest
}

type SearchRequest struct {
	Query string
	Limit int
}

type DiscoveryRequest struct {
	Query titlesearch.DiscoveryQuery
	Limit int
}

func (c *Corpus) Search(ctx context.Context, query string, limit int) ([]titlesearch.Candidate, error) {
	c.mu.Lock()
	c.searches = append(c.searches, SearchRequest{Query: query, Limit: limit})
	c.mu.Unlock()
	if c.SearchFunc != nil {
		return c.SearchFunc(ctx, query, limit)
	}
	return append([]titlesearch.Candidate(nil), c.Candidates...), nil
}

func (c *Corpus) Discover(ctx context.Context, query titlesearch.DiscoveryQuery, limit int) ([]titlesearch.Candidate, error) {
	c.mu.Lock()
	c.discoveries = append(c.discoveries, DiscoveryRequest{Query: query, Limit: limit})
	c.mu.Unlock()
	if c.DiscoverFunc != nil {
		return c.DiscoverFunc(ctx, query, limit)
	}
	return append([]titlesearch.Candidate(nil), c.Candidates...), nil
}

// ErrDiscover is a shared deterministic discovery-source failure for catalog
// tests that need to preserve errors.Is identity.
var ErrDiscover = errors.New("catalog fixture discovery failure")

func (c *Corpus) Searches() []SearchRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]SearchRequest(nil), c.searches...)
}

func (c *Corpus) Discoveries() []DiscoveryRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]DiscoveryRequest(nil), c.discoveries...)
}

// Library is a deterministic library-scope searcher returning fixed rows, for
// asserting how the catalog maps fields the pinned media-server fixture lacks.
type Library struct {
	Results []mediaserver.SearchResult
}

func (l *Library) Search(context.Context, string, int) ([]mediaserver.SearchResult, error) {
	return append([]mediaserver.SearchResult(nil), l.Results...), nil
}

// Presence is a deterministic library-ownership adapter.
type Presence struct {
	mu   sync.Mutex
	Hits map[int]titlesearch.Presence
	// Err, when set, fails every lookup, as an unreachable library store does.
	Err error
	// OnCall, when set, runs on every lookup before it answers (a test cancels its context here).
	OnCall func()
	calls  []int
}

func (p *Presence) Present(
	_ context.Context,
	_ provision.MediaType,
	tmdbID, _ int,
) (titlesearch.Presence, bool, error) {
	p.mu.Lock()
	p.calls = append(p.calls, tmdbID)
	hit, ok := p.Hits[tmdbID]
	err, onCall := p.Err, p.OnCall
	p.mu.Unlock()
	if onCall != nil {
		onCall()
	}
	if err != nil {
		return titlesearch.Presence{}, false, err
	}
	return hit, ok, nil
}

func (p *Presence) Calls() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.calls...)
}
