package channels_test

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/testkit"
)

// fakePods returns a fixed two-clip pod for any gap, recording that it was asked.
type fakePods struct {
	calls int
	seeds []int64
}

func (f *fakePods) FillGap(_ context.Context, channelID string, era int, gapMs, seed int64) []schedule.Slot {
	f.calls++
	f.seeds = append(f.seeds, seed)
	return []schedule.Slot{
		{Kind: schedule.SlotFiller, LibraryItemID: "clip-a", Title: "Frosted Flakes", DurationMs: 30000},
		{Kind: schedule.SlotFiller, LibraryItemID: "clip-b", Title: "TMNT figures", DurationMs: 30000},
	}
}

// With a PodFiller wired, a channel's filler gaps are replaced by matched pod
// clips during reconcile (§10). The clips carry library item ids → they render as
// real content on Tunarr, not flex.
func TestReconcile_PodsFillFillerGaps(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	// One available program + one pending → the pending becomes a pod-fill filler gap.
	avail := mapAvail{"movie:tmdb:1": "lib-1"}
	pods := &fakePods{}
	e := newEngine(st, tun, avail, nil).WithPods(pods)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))

	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if pods.calls == 0 {
		t.Fatal("PodFiller was never asked to fill the gap")
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	// The desired lineup now contains the pod's real clips (not an empty filler).
	var foundClipA, emptyFiller bool
	for _, s := range ch.Desired {
		if s.Kind == schedule.SlotFiller && s.LibraryItemID == "clip-a" {
			foundClipA = true
		}
		if s.Kind == schedule.SlotFiller && s.LibraryItemID == "" {
			emptyFiller = true
		}
	}
	if !foundClipA {
		t.Errorf("pod clips not placed in the lineup: %+v", ch.Desired)
	}
	if emptyFiller {
		t.Error("an unfilled (empty) filler slot remained — the pod should have filled it")
	}
	// The program slot is untouched (filler never displaces a program).
	if programCount(ch) != 1 {
		t.Errorf("program count changed: %+v", ch.Desired)
	}
}

// Determinism (§10): the same channel+slot yields the same seed across reconciles.
func TestReconcile_PodSeedDeterministic(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	pods := &fakePods{}
	e := newEngine(st, tun, mapAvail{}, nil).WithPods(pods)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:2", "B")) // all pending → one gap

	_ = e.Reconcile(context.Background(), "c1")
	firstSeeds := append([]int64{}, pods.seeds...)
	pods.seeds = nil
	_ = e.Reconcile(context.Background(), "c1")

	if len(firstSeeds) == 0 || len(pods.seeds) == 0 {
		t.Fatal("no pod seeds recorded")
	}
	if firstSeeds[0] != pods.seeds[0] {
		t.Errorf("pod seed not deterministic across reconciles: %d vs %d", firstSeeds[0], pods.seeds[0])
	}
}

// Without a PodFiller, the engine behaves exactly as Phase 10 (flex-only) — a
// filler gap stays an empty filler slot. This guards the opt-in seam.
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
}
