package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

func testDiscoveryFeedback(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()
	first := DiscoveryFeedback{ID: "f-1", ActorID: "admin", Scope: FeedbackHousehold,
		Target: provision.Key("movie:tmdb:603"), Action: FeedbackKeep, Reason: "family favorite", CreatedAt: base}
	if err := s.AppendDiscoveryFeedback(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendDiscoveryFeedback(ctx, DiscoveryFeedback{ID: "f-2", ActorID: "admin",
		Scope: FeedbackHousehold, Target: first.Target, Action: FeedbackNever, CreatedAt: base.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackHousehold})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "f-2" || got[1].Reason != "family favorite" {
		t.Fatalf("feedback events = %+v, want newest-first append-only history", got)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- s.AppendDiscoveryFeedback(ctx, DiscoveryFeedback{ID: fmt.Sprintf("parallel-%d", i),
				ActorID: "admin", Scope: FeedbackChannel, ScopeID: "channel-1",
				Target: provision.Key(fmt.Sprintf("movie:tmdb:%d", 100+i)), Action: FeedbackLess,
				CreatedAt: base.Add(2 * time.Second)})
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	parallel, err := s.ListDiscoveryFeedback(ctx, FeedbackFilter{Scope: FeedbackChannel, ScopeID: "channel-1"})
	if err != nil || len(parallel) != 8 {
		t.Fatalf("concurrent feedback = %d, %v; want 8 events", len(parallel), err)
	}
}
