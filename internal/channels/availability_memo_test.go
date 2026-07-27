package channels

import (
	"context"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// The N+1 these tests exist to prevent.
//
// `ComputeDesiredAt` walks a lineup repeatedly while laying out a cycle, and every pass used to
// re-resolve the same keys — each miss costing a fresh round-trip to the media server. On the
// maintainer's install ONE `GET /v1/guide` issued 72 library calls and spent 208ms–1.01s per
// channel inside ComputeDesiredAt, which was the whole ~2s the Guide took to appear.
//
// Nothing about the RESULT changed when that was fixed, which is exactly why these tests count
// CALLS rather than assert on returned values: the old code was correct and slow, so a
// value-only test passes identically before and after and guards nothing.

func availTestStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/avail.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func seedAvailable(t *testing.T, st store.Store, key provision.Key, libraryID string) {
	t.Helper()
	rec := provision.Record{
		Key: key, State: provision.Available, LibraryID: libraryID,
		Title:     provision.Title{Name: "Seeded"},
		UpdatedAt: time.Now(),
	}
	if err := st.UpsertTitle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func TestResolveMemoizesDurationPerLibraryItem(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("movie:tmdb:603")
	seedAvailable(t, st, key, "lib-603")

	calls := 0
	dur := func(_ context.Context, libraryItemID string) (int64, error) {
		calls++
		if libraryItemID != "lib-603" {
			t.Fatalf("duration asked for %q, want lib-603", libraryItemID)
		}
		return 8_160_000, nil
	}

	av := NewStoreAvailability(context.Background(), st, dur, nil)

	// Resolve the SAME key repeatedly, as a cycle layout does.
	for i := 0; i < 25; i++ {
		id, ms, ok := av.Resolve(key)
		if !ok || id != "lib-603" || ms != 8_160_000 {
			t.Fatalf("resolve %d = (%q, %d, %v), want (lib-603, 8160000, true)", i, id, ms, ok)
		}
	}

	if calls != 1 {
		t.Fatalf("duration resolver called %d times for one library item, want 1 (the N+1 is back)", calls)
	}
}

func TestResolveMemoizesTheStoreLookupIncludingMisses(t *testing.T) {
	st := availTestStore(t)
	present := provision.Key("movie:tmdb:603")
	absent := provision.Key("movie:tmdb:999999")
	seedAvailable(t, st, present, "lib-603")

	durCalls := 0
	dur := func(_ context.Context, _ string) (int64, error) { durCalls++; return 1, nil }
	av := NewStoreAvailability(context.Background(), st, dur, nil)

	for i := 0; i < 10; i++ {
		if _, _, ok := av.Resolve(present); !ok {
			t.Fatal("a seeded available title must resolve")
		}
		// A MISS must be memoized too: re-asking the store for a key that was absent a
		// moment ago costs exactly as much as asking for one that was present.
		if _, _, ok := av.Resolve(absent); ok {
			t.Fatal("an unseeded title must not resolve")
		}
	}
	if durCalls != 1 {
		t.Fatalf("duration resolver called %d times, want 1", durCalls)
	}
}

func TestResolveEpisodesMemoizesTheEpisodeEnumeration(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("series:tvdb:71663")
	seedAvailable(t, st, key, "lib-show")

	calls := 0
	eps := func(_ context.Context, showItemID string) ([]schedule.ResolvedProgram, error) {
		calls++
		if showItemID != "lib-show" {
			t.Fatalf("episodes asked for %q, want lib-show", showItemID)
		}
		return []schedule.ResolvedProgram{
			{LibraryItemID: "ep-1", DurationMs: 1_320_000},
			{LibraryItemID: "ep-2", DurationMs: 1_320_000},
		}, nil
	}

	av := NewStoreAvailability(context.Background(), st, nil, eps)

	for i := 0; i < 25; i++ {
		got, ok := av.ResolveEpisodes(key)
		if !ok || len(got) != 2 {
			t.Fatalf("resolveEpisodes %d = (%d eps, %v), want (2, true)", i, len(got), ok)
		}
	}

	// The most expensive library call there is — enumerating a show — must happen once.
	if calls != 1 {
		t.Fatalf("episode resolver called %d times for one show, want 1", calls)
	}
}

func TestResolveEpisodesMemoizesAnEmptyEnumeration(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("series:tvdb:71663")
	seedAvailable(t, st, key, "lib-show")

	calls := 0
	eps := func(_ context.Context, _ string) ([]schedule.ResolvedProgram, error) {
		calls++
		return nil, nil // a show with nothing enumerable
	}

	av := NewStoreAvailability(context.Background(), st, nil, eps)
	for i := 0; i < 10; i++ {
		if _, ok := av.ResolveEpisodes(key); ok {
			t.Fatal("a show with no episodes must not resolve")
		}
	}
	if calls != 1 {
		t.Fatalf("episode resolver called %d times for an EMPTY show, want 1 — the miss must be memoized too", calls)
	}
}

// The memo is shared mutable state and the guide resolves channels concurrently, so this must
// be race-free. Run under `-race` (make check does) this fails loudly if the mutex goes away.
func TestAvailabilityMemoIsSafeUnderConcurrentResolve(t *testing.T) {
	st := availTestStore(t)
	keys := []provision.Key{"movie:tmdb:1", "movie:tmdb:2", "movie:tmdb:3"}
	for i, k := range keys {
		seedAvailable(t, st, k, "lib-"+string(rune('a'+i)))
	}

	dur := func(_ context.Context, _ string) (int64, error) { return 1_000, nil }
	av := NewStoreAvailability(context.Background(), st, dur, nil)

	done := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 50; i++ {
				for _, k := range keys {
					av.Resolve(k)
				}
			}
		}()
	}
	for g := 0; g < 8; g++ {
		<-done
	}
}
