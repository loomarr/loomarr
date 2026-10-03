package channels_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tunarr/tunarrtest"
)

// Airing history must never re-arrange the window on air (#1674).
//
// Production shape: an internal-playout shuffle channel on the default 24 h rolling window,
// reconciled every few minutes. A viewer tuning in makes playout record the programme it
// resolves (the resolver's own AiringAt + RecordAiring pair below). Before the fix the next
// reconcile read that record, byRecency re-sorted the deck, and windowSlice cut the new deck at
// the same offset: the same wall-clock instant mapped to a different programme, so the guide
// and the picture parted company.

// runtimeAvail resolves movies and series episodes with explicit runtimes.
type runtimeAvail struct {
	movies map[provision.Key]schedule.ResolvedProgram
	series map[provision.Key][]schedule.ResolvedProgram
}

func (a runtimeAvail) Resolve(k provision.Key) (string, int64, bool) {
	p, ok := a.movies[k]
	return p.LibraryItemID, p.DurationMs, ok
}

func (a runtimeAvail) ResolveEpisodes(k provision.Key) schedule.EpisodeResolution {
	return schedule.EpisodeResolution{Programs: a.series[k]}
}

// recencyEngine is an internal-playout engine on a 24 h window with a clock the test moves.
func recencyEngine(st store.Store, avail schedule.Availability, now *time.Time) *channels.Engine {
	return channels.New(st, tunarrtest.NewTunarr(), avail, nil, channels.Config{
		ReconcileTTL:  10 * time.Minute,
		DefaultWindow: 24 * time.Hour,
		ResolvePlayoutBackendContext: func(context.Context) (string, error) {
			return schedule.PlayoutBackendInternal, nil
		},
	}, func() time.Time { return *now }, testkit.Logger())
}

func seedShuffleChannel(t *testing.T, st store.Store, id string, entries []schedule.LineupEntry) {
	t.Helper()
	ch := store.Channel{Lineup: entries}
	ch.ID, ch.Name, ch.Number, ch.Strategy, ch.Status = id, "Ch "+id, 9, schedule.Shuffle, schedule.StatusBuilding
	if _, err := st.SaveChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

func films(n int, runtime time.Duration) ([]schedule.LineupEntry, runtimeAvail) {
	avail := runtimeAvail{movies: map[provision.Key]schedule.ResolvedProgram{}}
	var entries []schedule.LineupEntry
	for i := 1; i <= n; i++ {
		k := provision.Key(fmt.Sprintf("movie:tmdb:%d", 100+i))
		avail.movies[k] = schedule.ResolvedProgram{LibraryItemID: fmt.Sprintf("film-%d", i), DurationMs: runtime.Milliseconds()}
		entries = append(entries, schedule.LineupEntry{Key: k, Title: fmt.Sprintf("Film %d", i), DurationMs: runtime.Milliseconds()})
	}
	return entries, avail
}

func reconcileDesired(t *testing.T, e *channels.Engine, st store.Store, id string) store.Channel {
	t.Helper()
	if err := e.Reconcile(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	ch, err := st.GetChannel(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

// tuneIn does what playoutResolver.AiringAt does when a viewer tunes in: resolve the programme on
// air from the accepted cycle and record its airing, stamped with the programme's start.
func tuneIn(t *testing.T, st store.Store, ch store.Channel, now time.Time) playout.Airing {
	t.Helper()
	airing := playout.AiringAt(ch.Desired, ch.PlayoutAnchor, now)
	if airing.Key == "" {
		t.Fatalf("nothing on air at %v", now)
	}
	if err := st.RecordAiring(context.Background(), ch.ID, airing.Key, airing.LibraryItemID, now.Add(-airing.Offset), now); err != nil {
		t.Fatal(err)
	}
	return airing
}

func programIDs(slots []schedule.Slot) []string {
	var out []string
	for _, s := range slots {
		if s.IsProgram() {
			out = append(out, s.LibraryItemID)
		}
	}
	return out
}

// A reconcile after a tune never changes a slot in the current window: the accepted cycle, and
// therefore the guide and the encoder walking it from the same anchor, stays put. Twenty 2 h films
// is a 40 h pool on a 24 h window, so the rotating slice is live (the #1670 repro shape).
func TestReconcile_TuneInDoesNotRearrangeCurrentWindow(t *testing.T) {
	st := newStore(t)
	entries, avail := films(20, 2*time.Hour)
	now := time.Unix(1_800_000_000, 0).UTC() // mid-window: 08:00 UTC
	e := recencyEngine(st, avail, &now)
	seedShuffleChannel(t, st, "rw", entries)

	before := reconcileDesired(t, e, st, "rw")
	onAir := tuneIn(t, st, before, now.Add(10*time.Minute))

	now = now.Add(15 * time.Minute) // the next reconcile sweep, same window
	after := reconcileDesired(t, e, st, "rw")
	if !slices.Equal(programIDs(before.Desired), programIDs(after.Desired)) {
		t.Fatalf("a tune-in re-arranged the window on air (tuned to %s):\n before %v\n after  %v",
			onAir.LibraryItemID, programIDs(before.Desired), programIDs(after.Desired))
	}
	if got := playout.AiringAt(after.Desired, after.PlayoutAnchor, now); got.LibraryItemID != onAir.LibraryItemID {
		t.Fatalf("programme on air changed from %s to %s after a reconcile", onAir.LibraryItemID, got.LibraryItemID)
	}

	// The cycle preview for the current window (the guide's and the editor's view) agrees.
	_, preview, _, _, err := e.CyclePreview(context.Background(), "rw", now)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(programIDs(before.Desired), programIDs(preview)) {
		t.Fatalf("cycle preview for the current window moved after a tune-in:\n accepted %v\n preview  %v",
			programIDs(before.Desired), programIDs(preview))
	}
}

// History is not dropped, only deferred: the NEXT window's arrangement reads it. A 6 h pool fits
// the 24 h window whole, so recency alone decides the order and the aired film goes last.
func TestReconcile_AiringHistoryAppliesAtTheNextWindow(t *testing.T) {
	st := newStore(t)
	entries, avail := films(6, time.Hour)
	now := time.Unix(1_800_000_000, 0).UTC()
	e := recencyEngine(st, avail, &now)
	seedShuffleChannel(t, st, "nw", entries)

	before := reconcileDesired(t, e, st, "nw")
	onAir := tuneIn(t, st, before, now)

	now = now.Add(5 * time.Minute)
	if got := programIDs(reconcileDesired(t, e, st, "nw").Desired); !slices.Equal(got, programIDs(before.Desired)) {
		t.Fatalf("same window re-arranged: before %v, after %v", programIDs(before.Desired), got)
	}

	now = now.Truncate(24 * time.Hour).Add(24*time.Hour + time.Minute) // just past the boundary
	next := programIDs(reconcileDesired(t, e, st, "nw").Desired)
	if next[len(next)-1] != onAir.LibraryItemID {
		t.Fatalf("next window should air the aired film (%s) last; got %v", onAir.LibraryItemID, next)
	}
}

// History is per episode, not per series: one aired episode moves back, its unaired siblings do
// not. Before the fix the record was keyed by the series, so a single tune-in pushed every episode
// of the watched show behind everything else.
func TestReconcile_AiringHistoryIsPerEpisode(t *testing.T) {
	st := newStore(t)
	avail := runtimeAvail{series: map[provision.Key][]schedule.ResolvedProgram{}}
	var entries []schedule.LineupEntry
	for s, key := range []provision.Key{"series:tvdb:501", "series:tvdb:502"} {
		for ep := 1; ep <= 4; ep++ {
			avail.series[key] = append(avail.series[key], schedule.ResolvedProgram{
				LibraryItemID: fmt.Sprintf("show%d-e%d", s+1, ep), DurationMs: (30 * time.Minute).Milliseconds(),
				Season: 1, Episode: ep,
			})
		}
		entries = append(entries, schedule.LineupEntry{Key: key, Title: fmt.Sprintf("Show %d", s+1)})
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	e := recencyEngine(st, avail, &now)
	seedShuffleChannel(t, st, "pe", entries)

	// One episode aired in an earlier window.
	aired := now.Add(-30 * time.Hour)
	if err := st.RecordAiring(context.Background(), "pe", "series:tvdb:501", "show1-e1", aired, aired); err != nil {
		t.Fatal(err)
	}
	got := programIDs(reconcileDesired(t, e, st, "pe").Desired)
	if len(got) != 8 {
		t.Fatalf("want 8 episodes, got %v", got)
	}
	if got[len(got)-1] != "show1-e1" {
		t.Fatalf("the aired episode should go last; got %v", got)
	}
	tail := got[len(got)-4:]
	allShow1 := true
	for _, id := range tail {
		allShow1 = allShow1 && id[:5] == "show1"
	}
	if allShow1 {
		t.Fatalf("one aired episode pushed its whole series to the back: %v", got)
	}
}
