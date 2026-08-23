package channels_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
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
	calls         int
	hasCalls      int
	durationCalls int
	seeds         []int64
	sels          []filler.Selection // the selection each call received
	ids           []string           // the pool to return; nil → ok=false (no filler)
	duration      int64
	fitChannels   map[string]bool
}

func (f *fakePods) FitForChannel(channelID string, _ filler.Selection, _ filler.Clip) filler.Fit {
	if f.fitChannels == nil || f.fitChannels[channelID] {
		return filler.Fit{}
	}
	return filler.Fit{Reason: filler.FitAudience}
}

// HasPool mirrors the real adapter: a pool exists when there are clips to play. The double
// keys on the same `ids` so a test that seeds clips gets breaks, and one that seeds none does
// not — the behaviour the gate actually depends on.
func (f *fakePods) HasPool(_ context.Context, _ string, _ int64, _ filler.Selection) bool {
	f.hasCalls++
	return len(f.ids) > 0
}

func (f *fakePods) PlayableDurationMs(_ context.Context, _ string, _ int64, _ filler.Selection) int64 {
	f.durationCalls++
	if f.duration > 0 {
		return f.duration
	}
	return int64(len(f.ids)) * 30_000
}

func (f *fakePods) BuildFillerList(_ context.Context, channelID string, seed int64, sel filler.Selection) ([]string, bool) {
	f.calls++
	f.seeds = append(f.seeds, seed)
	f.sels = append(f.sels, sel)
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

func TestReconcileFillerChange_TargetsOnlyCompatibleChannels(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	seedChannel(t, st, "fits", 5, entry("movie:tmdb:1", "A"))
	seedChannel(t, st, "does-not-fit", 6, entry("movie:tmdb:1", "A"))
	pods := &fakePods{ids: []string{"clip-a"}, fitChannels: map[string]bool{"fits": true}}
	e := newEngine(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, nil).WithPods(pods)

	if err := e.ReconcileFillerChange(context.Background(), []filler.Clip{{Hash: "clip-a", Kind: filler.Commercial}}); err != nil {
		t.Fatal(err)
	}
	fit, _ := st.GetChannel(context.Background(), "fits")
	notFit, _ := st.GetChannel(context.Background(), "does-not-fit")
	if len(fit.Desired) == 0 {
		t.Fatal("compatible channel was not reconciled")
	}
	if len(notFit.Desired) != 0 {
		t.Fatalf("incompatible channel was reconciled: %+v", notFit.Desired)
	}
}

// Internal playout fills breaks from local clip paths, not Tunarr program uuids. Reconcile
// must ask HasPool and persist the break gaps without ever building a Tunarr filler-list.
func TestReconcile_InternalUsesBackendIndependentFillerPool(t *testing.T) {
	st := newStore(t)
	avail := mapAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}
	pods := &fakePods{ids: []string{"local-clip"}}
	e := channels.New(st, nil, avail, nil, channels.Config{
		ReconcileTTL:  10 * time.Minute,
		BreaksPerHour: 30,
		ResolvePlayoutBackendContext: func(context.Context) (string, error) {
			return schedule.PlayoutBackendInternal, nil
		},
	}, func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, testkit.Logger()).WithPods(pods)
	seedChannel(t, st, "internal-pods", 5,
		entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))

	if err := e.Reconcile(context.Background(), "internal-pods"); err != nil {
		t.Fatal(err)
	}
	if pods.durationCalls != 1 {
		t.Fatalf("PlayableDurationMs calls = %d, want 1", pods.durationCalls)
	}
	if pods.calls != 0 {
		t.Fatalf("internal reconcile built a Tunarr filler-list %d times", pods.calls)
	}
	ch, err := st.GetChannel(context.Background(), "internal-pods")
	if err != nil {
		t.Fatal(err)
	}
	breaks := 0
	for _, slot := range ch.Desired {
		if slot.Kind == schedule.SlotFiller {
			breaks++
		}
	}
	if breaks == 0 {
		t.Fatalf("local filler pool did not materialize break gaps: %+v", ch.Desired)
	}
}

func TestReconcile_InternalBreakEndsWhenItsPodIsExhausted(t *testing.T) {
	st := newStore(t)
	avail := mapAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}
	pods := &fakePods{ids: []string{"commercial-a", "commercial-b"}, duration: 40_000}
	e := channels.New(st, nil, avail, nil, channels.Config{
		ReconcileTTL:  10 * time.Minute,
		BreaksPerHour: 30,
		BreakDuration: 5 * time.Minute,
		ResolvePlayoutBackendContext: func(context.Context) (string, error) {
			return schedule.PlayoutBackendInternal, nil
		},
	}, func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, testkit.Logger()).WithPods(pods)
	seedChannel(t, st, "underfilled", 5,
		entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))

	if err := e.Reconcile(context.Background(), "underfilled"); err != nil {
		t.Fatal(err)
	}
	ch, err := st.GetChannel(context.Background(), "underfilled")
	if err != nil {
		t.Fatal(err)
	}
	_, preview, _, _, err := e.CyclePreview(context.Background(), "underfilled", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range preview {
		if slot.Kind == schedule.SlotFiller && slot.Key == "" && slot.DurationMs != 40_000 {
			t.Fatalf("preview break duration = %dms, want accepted duration 40000ms", slot.DurationMs)
		}
	}
	var beforeBreak time.Duration
	for _, slot := range ch.Desired {
		if slot.Kind == schedule.SlotFiller && slot.Key == "" {
			if slot.DurationMs != 40_000 {
				t.Fatalf("commercial break duration = %dms, want the two configured clips' 40000ms", slot.DurationMs)
			}
			atBoundary := playout.AiringAt(ch.Desired, ch.PlayoutAnchor,
				ch.PlayoutAnchor.Add(beforeBreak+40*time.Second))
			if atBoundary.Kind != schedule.SlotProgram || atBoundary.LibraryItemID != "lib-2" {
				t.Fatalf("airing after commercial two = %+v, want the following programme", atBoundary)
			}
			return
		}
		beforeBreak += time.Duration(slot.DurationMs) * time.Millisecond
	}
	t.Fatal("no commercial break was interleaved")
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

// Breaks are only interleaved when a filler pool exists (the empty-filler fix):
// inserting commercial-break gaps with no clips to fill them leaves empty flex that
// Tunarr renders as large channel-named blocks in the guide. With BreaksPerHour set,
// a channel with a pool gets break slots between its programs; the SAME channel with
// an empty catalog gets NONE — its programs play back-to-back. Self-heals once clips land.
func TestReconcile_NoBreaksWhenNoFillerPool(t *testing.T) {
	// Two AVAILABLE programs so interleaveBreaks has a gap to fill (a single program
	// never gets a trailing break). BreaksPerHour high enough to force a break between them.
	seed := func() (store.Store, *testkit.Tunarr) {
		st := newStore(t)
		tun := testkit.NewTunarr()
		seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))
		return st, tun
	}
	avail := mapAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}
	breaks := func(st store.Store, tun *testkit.Tunarr, pods *fakePods) *channels.Engine {
		e := channels.New(st, tun, avail, nil,
			channels.Config{ReconcileTTL: 10 * time.Minute, BreaksPerHour: 30}, // 30/hr → a break between hour-scale gaps
			func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, testkit.Logger())
		if pods != nil {
			e = e.WithPods(pods)
		}
		return e
	}
	countBreaks := func(ch store.Channel) int {
		n := 0
		for _, s := range ch.Desired {
			if s.Kind == schedule.SlotFiller {
				n++
			}
		}
		return n
	}

	// With a real pool: breaks ARE interleaved (filler slots present).
	stP, tunP := seed()
	if err := breaks(stP, tunP, &fakePods{ids: []string{"clip-a"}}).Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	chP, _ := stP.GetChannel(context.Background(), "c1")
	withPool := countBreaks(chP)
	if withPool == 0 {
		t.Fatal("with a filler pool + BreaksPerHour, expected break slots between programs, got none")
	}

	// With an empty catalog (ok=false): NO breaks — programs play back-to-back, no empty flex.
	stE, tunE := seed()
	if err := breaks(stE, tunE, &fakePods{}).Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	chE, _ := stE.GetChannel(context.Background(), "c1")
	if got := countBreaks(chE); got != 0 {
		t.Errorf("empty catalog must insert NO break slots (else empty flex → channel-named guide blocks), got %d", got)
	}

	// And with no PodFiller at all (nil pods): also no breaks.
	stN, tunN := seed()
	if err := breaks(stN, tunN, nil).Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	chN, _ := stN.GetChannel(context.Background(), "c1")
	if got := countBreaks(chN); got != 0 {
		t.Errorf("no PodFiller must insert NO break slots, got %d", got)
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
