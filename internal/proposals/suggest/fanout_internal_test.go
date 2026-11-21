package suggest

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/proposals"
)

// The bound is on in-flight work, not on how many items there are: 64 items must run, all of
// them, while never more than maxCatalogFanOut at once, and the bound must actually be reached
// (a limit of 1 would also "never exceed 8").
func TestForEachBoundedNeverExceedsTheLimitAndRunsEveryItem(t *testing.T) {
	const items = 64
	var inFlight, peak, ran atomic.Int64
	release := make(chan struct{})
	var once sync.Once
	var reached atomic.Int64

	done := make(chan struct{})
	go func() {
		defer close(done)
		forEachBounded(items, func(int) {
			now := inFlight.Add(1)
			for {
				seen := peak.Load()
				if now <= seen || peak.CompareAndSwap(seen, now) {
					break
				}
			}
			if reached.Add(1) == maxCatalogFanOut {
				once.Do(func() { close(release) })
			}
			<-release
			time.Sleep(time.Millisecond) // give an over-eager scheduler room to overshoot
			ran.Add(1)
			inFlight.Add(-1)
		})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("forEachBounded did not finish")
	}
	if got := ran.Load(); got != items {
		t.Fatalf("ran %d items, want %d", got, items)
	}
	if got := peak.Load(); got != maxCatalogFanOut {
		t.Fatalf("peak concurrency = %d, want exactly %d", got, maxCatalogFanOut)
	}
}

// overlapCountingCorpus is a TMDB-scope corpus that records how many searches overlap.
type overlapCountingCorpus struct {
	inFlight, peak *atomic.Int64
}

func (c overlapCountingCorpus) Search(context.Context, string, int) ([]titlesearch.Candidate, error) {
	now := c.inFlight.Add(1)
	defer c.inFlight.Add(-1)
	for {
		seen := c.peak.Load()
		if now <= seen || c.peak.CompareAndSwap(seen, now) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond)
	return nil, nil
}

// The two grounding passes go through the bound: a Catalog that counts overlapping searches
// never sees more than the limit for an explicit-title pass.
func TestGroundExplicitRequiredTitlesSearchesThroughTheBound(t *testing.T) {
	var inFlight, peak atomic.Int64
	corpus := overlapCountingCorpus{inFlight: &inFlight, peak: &peak}
	s := New(nil, titlesearch.New(nil, corpus), nil, 10)
	intent := Intent{Intent: proposals.Intent{MustInclude: []string{
		"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel",
	}}}
	if _, err := s.groundExplicitRequiredTitles(t.Context(), &intent, ValidatedDateMeaning{}); err != nil {
		t.Fatal(err)
	}
	if got := peak.Load(); got < 1 || got > maxCatalogFanOut {
		t.Fatalf("peak overlapping catalog searches = %d, want 1..%d", got, maxCatalogFanOut)
	}
}
