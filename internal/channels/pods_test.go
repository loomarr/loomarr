package channels_test

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// chTunarrID returns the server-assigned Tunarr id a reconcile persisted for a
// channel (so tests can look up its attached filler list).
func chTunarrID(t *testing.T, st store.Store, id string) string {
	t.Helper()
	ch, err := st.GetChannel(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ch.TunarrID
}

// fakePods returns a fixed two-clip pool for a channel, recording the seed it was
// asked with (§10 redesign: BuildFillerList returns program uuids for the channel's
// Tunarr filler-list, not per-gap slots).
type fakePods struct {
	calls int
	seeds []int64
	ids   []string // the pool to return; nil → ok=false (no filler)
}

func (f *fakePods) BuildFillerList(_ context.Context, channelID string, era int, seed int64) ([]string, bool) {
	f.calls++
	f.seeds = append(f.seeds, seed)
	if len(f.ids) == 0 {
		return nil, false
	}
	return f.ids, true
}

// With a PodFiller wired, a channel's matched clip pool is built and attached to
// Tunarr as a filler-list during reconcile (§10 redesign). The interleaved break
// gaps stay FLEX in the pushed lineup (Tunarr fills them from the list) — they are
// NOT inline-expanded into content slots.
func TestReconcile_PodsBuildAndAttachFillerList(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1"}
	pods := &fakePods{ids: []string{"clip-a", "clip-b"}}
	e := newEngine(st, tun, avail, nil).WithPods(pods)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))

	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if pods.calls == 0 {
		t.Fatal("PodFiller was never asked to build the filler list")
	}
	// The filler-list was attached to Tunarr with the matched pool.
	if got := tun.FillerListFor(chTunarrID(t, st, "c1")); len(got) != 2 || got[0] != "clip-a" {
		t.Errorf("filler list not attached with the pool: %v", got)
	}
	// The desired lineup carries NO inline filler content — filler stays flex/empty
	// (Tunarr plays the filler-list into it).
	ch, _ := st.GetChannel(context.Background(), "c1")
	for _, s := range ch.Desired {
		if s.Kind == schedule.SlotFiller && s.LibraryItemID != "" {
			t.Errorf("filler was inline-expanded into content (should be flex): %+v", s)
		}
	}
	// The program slot is untouched (filler never displaces a program).
	if programCount(ch) != 1 {
		t.Errorf("program count changed: %+v", ch.Desired)
	}
}

// Idempotency (§9/§10): a second reconcile with an unchanged pool makes NO new
// filler-list write — the contents aren't part of the lineup diff, so the attach
// must be internally idempotent or it churns Tunarr every sweep.
func TestReconcile_FillerListIdempotent(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	pods := &fakePods{ids: []string{"clip-a", "clip-b"}}
	e := newEngine(st, tun, mapAvail{}, nil).WithPods(pods)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:2", "B"))

	_ = e.Reconcile(context.Background(), "c1")
	firstWrites := tun.FillerWrites
	_ = e.Reconcile(context.Background(), "c1")
	if tun.FillerWrites != firstWrites {
		t.Errorf("second reconcile re-wrote an unchanged filler list: %d → %d", firstWrites, tun.FillerWrites)
	}
	// And the seed is deterministic across reconciles (same pool rebuilds).
	if len(pods.seeds) < 2 || pods.seeds[0] != pods.seeds[1] {
		t.Errorf("filler seed not deterministic across reconciles: %v", pods.seeds)
	}
}

// An empty catalog (BuildFillerList ok=false) attaches no filler list — the channel
// falls back to flex / the bumper card, never dead air (§10).
func TestReconcile_EmptyCatalogNoFillerList(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	pods := &fakePods{} // ids nil → ok=false
	e := newEngine(st, tun, mapAvail{}, nil).WithPods(pods)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:2", "B"))

	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if got := tun.FillerListFor(chTunarrID(t, st, "c1")); len(got) != 0 {
		t.Errorf("empty catalog should attach no filler list, got %v", got)
	}
	// The break gaps remain empty filler (flex) in the desired lineup.
	ch, _ := st.GetChannel(context.Background(), "c1")
	var emptyFiller bool
	for _, s := range ch.Desired {
		if s.Kind == schedule.SlotFiller && s.LibraryItemID == "" {
			emptyFiller = true
		}
	}
	if !emptyFiller {
		t.Error("break gap should remain an empty (flex) filler slot")
	}
}

// Without a PodFiller, the engine behaves exactly as Phase 10 (flex-only) — a
// filler gap stays an empty filler slot and no filler-list is attached.
func TestReconcile_NoPodFillerLeavesFlex(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	e := newEngine(st, tun, mapAvail{}, nil) // no .WithPods
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:2", "B"))

	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	var emptyFiller bool
	for _, s := range ch.Desired {
		if s.Kind == schedule.SlotFiller && s.LibraryItemID == "" {
			emptyFiller = true
		}
	}
	if !emptyFiller {
		t.Error("without a PodFiller the gap should remain an empty (flex) filler slot")
	}
	if tun.FillerWrites != 0 {
		t.Errorf("no PodFiller → no filler-list writes, got %d", tun.FillerWrites)
	}
}
