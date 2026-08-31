package channels

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
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
		got := av.ResolveEpisodes(key)
		if len(got.Programs) != 2 {
			t.Fatalf("resolveEpisodes %d = %d eps, want 2", i, len(got.Programs))
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
		if got := av.ResolveEpisodes(key); len(got.Programs) > 0 {
			t.Fatal("a show with no episodes must not resolve")
		}
	}
	if calls != 1 {
		t.Fatalf("episode resolver called %d times for an EMPTY show, want 1 — the miss must be memoized too", calls)
	}
}

// The memo EXPIRES. This is the correctness half, and it needs its own test because every
// assertion above would pass with no TTL at all.
//
// It matters because the composition root builds ONE storeAvailability at boot with the
// process-lifetime rootCtx (app.go) — the memo lives as long as the process. Without an expiry
// a title that finished acquiring would read `pending` forever, and the scheduler would keep
// laying out a pending slot for content that had already landed.
func TestMemoExpiresSoAvailabilityCanChange(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("movie:tmdb:603")
	seedAvailable(t, st, key, "lib-603")

	clock := time.Now()
	calls := 0
	dur := func(_ context.Context, _ string) (int64, error) { calls++; return 1_000, nil }

	av := NewStoreAvailability(context.Background(), st, dur, nil)
	// Drive the clock rather than sleeping: the expiry is a property of the code, and a test
	// that sleeps for it is slow AND flaky.
	av.(*storeAvailability).now = func() time.Time { return clock }

	av.Resolve(key)
	av.Resolve(key)
	if calls != 1 {
		t.Fatalf("within the TTL: %d calls, want 1", calls)
	}

	// Past the TTL, the next resolve must go back to the source.
	//
	// The advance is a FIXED 30s, deliberately not `memoTTL + something`: expressing it in
	// terms of the constant makes the test move with it, so raising memoTTL to 100h would
	// still pass and the expiry would be unguarded. (Verified: with the relative form, that
	// exact sabotage went green.) 30s also pins the intended ORDER OF MAGNITUDE — the memo is
	// meant to collapse repeats within one layout, not to be a cache that outlives a reconcile.
	clock = clock.Add(30 * time.Second)
	av.Resolve(key)
	if calls != 2 {
		t.Fatalf("30s after the first resolve: %d calls, want 2 — the memo must expire, or an acquisition landing is never seen (is memoTTL too long?)", calls)
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

// THE GAP THIS CLOSES: the per-item DurationResolver is an N+1 the scheduler cannot batch on
// its own — ComputeDesiredAt asks Resolve(key) one key at a time. A 25-movie channel therefore
// issued 25 sequential media-server calls per layout, which measured as ~375ms of GET /v1/guide's
// cold latency against a Cloudflare-fronted Emby (the arrangement itself is microseconds).
func TestPrewarmDurations_CollapsesTheNPlusOneToOneCall(t *testing.T) {
	st := availTestStore(t)

	const n = 25
	keys := make([]provision.Key, n)
	ids := make([]string, n)
	for i := range keys {
		keys[i] = provision.Key(fmt.Sprintf("movie:tmdb:%d", 1000+i))
		ids[i] = fmt.Sprintf("lib-%d", 1000+i)
		seedAvailable(t, st, keys[i], ids[i])
	}

	perItem := 0
	dur := func(_ context.Context, _ string) (int64, error) { perItem++; return 111, nil }

	bulkCalls := 0
	bulk := func(_ context.Context, itemIDs []string) (map[string]int64, error) {
		bulkCalls++
		out := make(map[string]int64, len(itemIDs))
		for _, id := range itemIDs {
			out[id] = 222
		}
		return out, nil
	}

	av := WithBulkDurations(NewStoreAvailability(context.Background(), st, dur, nil), bulk)
	av.(interface{ PrewarmDurations([]string) }).PrewarmDurations(ids)

	for _, k := range keys {
		if _, ms, ok := av.Resolve(k); !ok || ms != 222 {
			t.Fatalf("resolve %s = (%d, %v), want (222, true) — the prewarmed value", k, ms, ok)
		}
	}

	if bulkCalls != 1 {
		t.Fatalf("bulk resolver called %d times, want exactly 1", bulkCalls)
	}
	if perItem != 0 {
		t.Fatalf("per-item resolver called %d times after a prewarm, want 0 (the N+1 is back)", perItem)
	}
}

// A bulk failure must never make a layout fail or lose a title — the per-item path still answers
// every key. Duration is a refinement (ComputeDesired falls back to the entry's own), never a
// precondition.
func TestPrewarmDurations_BulkFailureFallsBackToPerItem(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("movie:tmdb:603")
	seedAvailable(t, st, key, "lib-603")

	perItem := 0
	dur := func(_ context.Context, _ string) (int64, error) { perItem++; return 8_160_000, nil }
	bulk := func(_ context.Context, _ []string) (map[string]int64, error) {
		return nil, errors.New("media server unreachable")
	}

	av := WithBulkDurations(NewStoreAvailability(context.Background(), st, dur, nil), bulk)
	av.(interface{ PrewarmDurations([]string) }).PrewarmDurations([]string{"lib-603"})

	id, ms, ok := av.Resolve(key)
	if !ok || id != "lib-603" || ms != 8_160_000 {
		t.Fatalf("resolve = (%q, %d, %v), want the per-item answer after a bulk failure", id, ms, ok)
	}
	if perItem != 1 {
		t.Fatalf("per-item resolver called %d times, want 1 (the fallback must still run)", perItem)
	}
}

// An id the bulk call could not answer must be memoized as a MISS, not left to fall through to a
// per-item call on every occurrence — that would re-introduce the N+1 for exactly the items the
// bulk answer was thinnest on.
func TestPrewarmDurations_UnansweredIDsDoNotFallBackPerOccurrence(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("movie:tmdb:777")
	seedAvailable(t, st, key, "lib-777")

	perItem := 0
	dur := func(_ context.Context, _ string) (int64, error) { perItem++; return 5, nil }
	// Succeeds, but returns nothing for the requested id.
	bulk := func(_ context.Context, _ []string) (map[string]int64, error) {
		return map[string]int64{}, nil
	}

	av := WithBulkDurations(NewStoreAvailability(context.Background(), st, dur, nil), bulk)
	av.(interface{ PrewarmDurations([]string) }).PrewarmDurations([]string{"lib-777"})

	for i := 0; i < 10; i++ {
		if _, _, ok := av.Resolve(key); !ok {
			t.Fatal("an available title must still resolve when its duration is unknown")
		}
	}
	if perItem != 0 {
		t.Fatalf("per-item resolver called %d times for an unanswered id, want 0", perItem)
	}
}
