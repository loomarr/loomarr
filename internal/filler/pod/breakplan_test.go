package pod_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

type memoryBreaks struct {
	byKey map[string]pod.StoredBreak
	saves int
}

func newMemoryBreaks() *memoryBreaks { return &memoryBreaks{byKey: map[string]pod.StoredBreak{}} }

func key(channel string, start time.Time) string { return channel + "@" + start.UTC().String() }

func (m *memoryBreaks) SaveBreaks(_ context.Context, breaks []pod.StoredBreak) error {
	m.saves += len(breaks)
	for _, b := range breaks {
		m.byKey[key(b.ChannelID, b.Start)] = b
	}
	return nil
}

func (m *memoryBreaks) LoadBreaks(_ context.Context, channelID string, from, to time.Time) ([]pod.StoredBreak, error) {
	var out []pod.StoredBreak
	for _, b := range m.byKey {
		if b.ChannelID == channelID && !b.Start.Before(from) && !b.Start.After(to) {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

func (m *memoryBreaks) KeepBreaks(_ context.Context, channelID string, from, until time.Time, starts []time.Time) error {
	keep := map[string]bool{}
	for _, s := range starts {
		keep[key(channelID, s)] = true
	}
	for k, b := range m.byKey {
		inRange := (from.IsZero() || !b.Start.Before(from)) && (until.IsZero() || b.Start.Before(until))
		if b.ChannelID == channelID && inRange && !keep[k] {
			delete(m.byKey, k)
		}
	}
	return nil
}

func planner(t *testing.T, catalog []pod.Clip, store *memoryBreaks) *pod.Adapter {
	t.Helper()
	return pod.NewAdapter(stubCatalog{clips: catalog}, &stubExposures{}, func() pod.Policy {
		return pod.Policy{BreakDurationMs: 120_000, PodMax: 4}
	}, discardLogger()).WithBreaks(store).WithClock(func() time.Time { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) })
}

var (
	planFrom = time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC)
	// fromStart plans from planFrom with nothing on air yet, clearing the whole channel.
	fromStart = pod.PlanBounds{From: planFrom, FixedBefore: planFrom}
)

func plansOf(n int, gap, ceiling time.Duration) []pod.BreakPlan {
	out := make([]pod.BreakPlan, n)
	for i := range out {
		out[i] = pod.BreakPlan{Gap: gap, Ceiling: ceiling}
		if i == 0 {
			out[i].Gap = 0
		}
	}
	return out
}

// A break ends when its ads do: with three 30 s ads and a station ID, each 2 minute slot airs
// 1m35s. The later breaks, in the same evening, repeat the ads seen longest ago rather than
// shrinking to the station ID. Each break's length is the media its stored pod holds, and the
// next break starts when it ends.
func TestPlanBreaksSizeEachBreakToItsStoredPod(t *testing.T) {
	store := newMemoryBreaks()
	adapter := planner(t, append(catalogOf(30, 30, 30), pod.Clip{Hash: "ident", Path: "ident.mp4",
		Role: pod.StationID, Era: 1992, Audience: pod.General, DurationMs: 5_000}), store)

	lengths, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart,
		plansOf(3, 30*time.Minute, 2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if lengths[0] < 90*time.Second || lengths[0] > 2*time.Minute {
		t.Errorf("first break = %v, want the three ads (and bookends) within the 2 minute ceiling", lengths[0])
	}
	if lengths[1] != lengths[0] || lengths[2] != lengths[0] {
		t.Errorf("later breaks = %v, %v, want %v: a thin set repeats its oldest ads, it does not shrink the break",
			lengths[1], lengths[2], lengths[0])
	}
	stored, _ := store.LoadBreaks(context.Background(), "ch-1", planFrom, planFrom.Add(24*time.Hour))
	wantStarts := []time.Time{planFrom, planFrom.Add(lengths[0] + 30*time.Minute),
		planFrom.Add(lengths[0] + 30*time.Minute + lengths[1] + 30*time.Minute)}
	if len(stored) != 3 {
		t.Fatalf("stored %d breaks, want 3", len(stored))
	}
	for i, b := range stored {
		if !b.Start.Equal(wantStarts[i]) || b.Length() != lengths[i] {
			t.Errorf("break %d stored at %v with length %v, want %v and %v", i, b.Start, b.Length(), wantStarts[i], lengths[i])
		}
	}
}

// Planning again changes nothing: a stored break keeps its pod and length, and nothing is saved
// twice. A break whose start moved has nothing stored and is assembled fresh, and the break left at
// its old start is deleted.
func TestPlanBreaksAssembleOnlyWhatMoved(t *testing.T) {
	store := newMemoryBreaks()
	adapter := planner(t, catalogOf(30, 30, 30, 30, 30, 30, 30, 30), store)
	plans := plansOf(3, 30*time.Minute, 2*time.Minute)

	first, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart, plans)
	if err != nil {
		t.Fatal(err)
	}
	saved := store.saves
	again, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart, plans)
	if err != nil {
		t.Fatal(err)
	}
	if store.saves != saved || again[0] != first[0] || again[1] != first[1] || again[2] != first[2] {
		t.Fatalf("a second plan saved %d more breaks and returned %v, want nothing new and %v", store.saves-saved, again, first)
	}

	// A lineup edit that moves the first programme by a minute moves every break after it.
	moved := plansOf(3, 30*time.Minute, 2*time.Minute)
	moved[0].Gap = time.Minute
	if _, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart, moved); err != nil {
		t.Fatal(err)
	}
	if store.saves != saved+3 {
		t.Errorf("moved breaks saved %d, want all 3 assembled fresh", store.saves-saved)
	}
	if left, _ := store.LoadBreaks(context.Background(), "ch-1", planFrom, planFrom); len(left) != 0 {
		t.Errorf("the break at the old start is still stored")
	}
}

// A break stored for another ceiling is assembled again: a shorter break setting must not leave
// a stored pod longer than its slot, and a longer one gets the room it was given.
func TestPlanBreaksReassembleWhenTheCeilingChanges(t *testing.T) {
	store := newMemoryBreaks()
	adapter := planner(t, catalogOf(30, 30, 30, 30, 30, 30, 30, 30), store)
	if _, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart,
		plansOf(1, 0, 2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart,
		plansOf(1, 0, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got[0] > time.Minute {
		t.Fatalf("after the ceiling fell to a minute the break is %v", got[0])
	}
	stored, _ := store.LoadBreaks(context.Background(), "ch-1", planFrom, planFrom)
	if len(stored) != 1 || stored[0].Ceiling != time.Minute {
		t.Fatalf("stored %+v, want the break re-stored at the one minute ceiling", stored)
	}
}

// The window on air and the next, planned ahead from the turn, share a channel: the first stops at
// the turn and the second clears only from it, so planning either again keeps the other's breaks.
// The playout reads the next window's lengths without assembling anything.
func TestPlanBreaksTwoWindowsShareAChannel(t *testing.T) {
	store := newMemoryBreaks()
	adapter := planner(t, append(catalogOf(30, 30, 30, 30), pod.Clip{Hash: "ident", Path: "ident.mp4",
		Role: pod.StationID, Era: 1992, Audience: pod.General, DurationMs: 5_000}), store)
	turn := planFrom.Add(45 * time.Minute)
	current := pod.PlanBounds{From: planFrom, FixedBefore: planFrom, Until: turn}
	next := pod.PlanBounds{From: turn.Add(20 * time.Minute), FixedBefore: planFrom, ClearFrom: turn}
	// The current cycle runs past the turn: its third break, an hour in, lies beyond it.
	currentPlans := plansOf(3, 30*time.Minute, 2*time.Minute)
	nextPlans := plansOf(2, 30*time.Minute, 2*time.Minute)

	for range 2 { // planning each again must not disturb the other
		if _, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, current, currentPlans); err != nil {
			t.Fatal(err)
		}
		if _, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, next, nextPlans); err != nil {
			t.Fatal(err)
		}
	}
	saves := store.saves
	if _, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, current, currentPlans); err != nil {
		t.Fatal(err)
	}
	if store.saves != saves {
		t.Errorf("planning the window on air again assembled %d breaks: the plans are deleting each other's", store.saves-saves)
	}
	all, _ := store.LoadBreaks(context.Background(), "ch-1", planFrom, planFrom.Add(4*time.Hour))
	var before, after int
	for _, b := range all {
		if b.Start.Before(turn) {
			before++
		} else {
			after++
		}
	}
	if before != 2 || after != 2 {
		t.Fatalf("stored %d breaks before the turn and %d after, want the current window's two and the next's two", before, after)
	}

	planned, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, next, nextPlans)
	if err != nil {
		t.Fatal(err)
	}
	saves = store.saves
	read, err := adapter.PlannedLengths(context.Background(), "ch-1", next.From, nextPlans)
	if err != nil {
		t.Fatal(err)
	}
	if store.saves != saves || len(read) != 2 || read[0] != planned[0] || read[1] != planned[1] {
		t.Fatalf("read lengths %v (saving %d), want the planned %v and no writes", read, store.saves-saves, planned)
	}
	if read[1] >= 2*time.Minute {
		t.Errorf("second break of the next window = %v, want it cut to its pod for this test to mean anything", read[1])
	}
}

// A break already on air is fixed: it keeps the length it was stored with, so planning later in
// the evening walks the same timeline as planning before it aired. A fixed break that was never
// stored aired at its slot's length and is not assembled now, after the fact.
func TestPlanBreaksKeepsAFixedBreakAsItAired(t *testing.T) {
	store := newMemoryBreaks()
	adapter := planner(t, append(catalogOf(30, 30), pod.Clip{Hash: "ident", Path: "ident.mp4",
		Role: pod.StationID, Era: 1992, Audience: pod.General, DurationMs: 5_000}), store)
	plans := plansOf(2, 30*time.Minute, 2*time.Minute)
	before, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart, plans)
	if err != nil {
		t.Fatal(err)
	}
	if before[0] >= 2*time.Minute {
		t.Fatalf("first break = %v, want it shorter than its ceiling for this test to mean anything", before[0])
	}

	// Planning again just after the first break started: it is fixed by the length it was stored with.
	after, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, pod.PlanBounds{From: planFrom, FixedBefore: planFrom.Add(time.Second)}, plans)
	if err != nil {
		t.Fatal(err)
	}
	if after[0] != before[0] || after[1] != before[1] {
		t.Fatalf("after the first break aired, lengths = %v, want the planned %v", after, before)
	}

	fresh := newMemoryBreaks()
	unplanned := planner(t, catalogOf(30, 30, 30, 30), fresh)
	got, err := unplanned.PlanBreaks(context.Background(), "ch-2", pod.Selection{}, nil, pod.PlanBounds{From: planFrom, FixedBefore: planFrom.Add(time.Second)},
		[]pod.BreakPlan{{Ceiling: 2 * time.Minute}, {Gap: 30 * time.Minute, Ceiling: 2 * time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 2*time.Minute {
		t.Errorf("a fixed break nobody stored = %v, want its 2 minute slot", got[0])
	}
	if stored, _ := fresh.LoadBreaks(context.Background(), "ch-2", planFrom, planFrom); len(stored) != 0 {
		t.Errorf("a fixed break was assembled after it aired")
	}
	if stored, _ := fresh.LoadBreaks(context.Background(), "ch-2", planFrom.Add(32*time.Minute), planFrom.Add(32*time.Minute)); len(stored) != 1 {
		t.Errorf("the break still to come was not planned after the fixed one's full slot")
	}
}

// The guide reads the stored pod: asked for a planned break, the previewer returns what was
// planned, not a second assembly.
func TestPreviewAtManyReadsTheStoredBreak(t *testing.T) {
	store := newMemoryBreaks()
	adapter := planner(t, catalogOf(30, 30, 30, 30, 30, 30), store)
	if _, err := adapter.PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart,
		plansOf(2, 30*time.Minute, 2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	stored, _ := store.LoadBreaks(context.Background(), "ch-1", planFrom, planFrom)
	got, err := adapter.PreviewAtMany(context.Background(), "ch-1", pod.Selection{},
		[]pod.Break{{Seed: 999, Start: planFrom, Length: 2 * time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Entries) != len(stored[0].Pod.Entries) {
		t.Fatalf("preview has %d entries, stored break has %d", len(got[0].Entries), len(stored[0].Pod.Entries))
	}
	for i, e := range got[0].Entries {
		if e.Hash != stored[0].Pod.Entries[i].Hash {
			t.Fatalf("entry %d = %s, want the stored %s", i, e.Hash, stored[0].Pod.Entries[i].Hash)
		}
	}
}

// An install with no stored breaks plays as today: the previewer assembles live until reconcile
// has planned them.
func TestPreviewAtManyAssemblesLiveWithoutAStoredBreak(t *testing.T) {
	adapter := planner(t, catalogOf(30, 30, 30, 30), newMemoryBreaks())
	got, err := adapter.PreviewAtMany(context.Background(), "ch-1", pod.Selection{},
		[]pod.Break{{Seed: 5, Start: planFrom, Length: 2 * time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Entries) == 0 {
		t.Fatal("no stored break and no live assembly: the break is empty")
	}
}

// A break stored with no ad is filled once ads are ready, before it airs; while there are still
// none, planning again stores nothing.
func TestPlanBreaksFillsAnEmptyBreakOnceAdsAreReady(t *testing.T) {
	ident := pod.Clip{Hash: "ident", Path: "ident.mp4", Role: pod.StationID, Era: 1992,
		Audience: pod.General, DurationMs: 5_000}
	store := newMemoryBreaks()
	plans := plansOf(2, 30*time.Minute, 2*time.Minute)

	empty, err := planner(t, []pod.Clip{ident}, store).PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart, plans)
	if err != nil {
		t.Fatal(err)
	}
	if empty[0] != 5*time.Second {
		t.Fatalf("with no ads the break = %v, want the station ID alone", empty[0])
	}
	saves := store.saves
	if _, err := planner(t, []pod.Clip{ident}, store).PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart, plans); err != nil {
		t.Fatal(err)
	}
	if store.saves != saves {
		t.Errorf("planning again with still no ads saved %d breaks, want none", store.saves-saves)
	}

	filled, err := planner(t, append(catalogOf(30, 30, 30), ident), store).PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart, plans)
	if err != nil {
		t.Fatal(err)
	}
	if filled[0] < 90*time.Second {
		t.Errorf("once ads are ready the break = %v, want the ads in it", filled[0])
	}
}

// A break books each ad by its air window (#3384), carries where the window starts, and plans
// again once a window is measured, so a stored break never keeps airing the dead air it left out.
func TestPlanBreaksBooksAdsByTheirAirWindow(t *testing.T) {
	catalog := catalogOf(30, 30, 30, 30)
	store := newMemoryBreaks()
	plans := plansOf(1, 0, 2*time.Minute)

	whole, err := planner(t, catalog, store).PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart, plans)
	if err != nil {
		t.Fatal(err)
	}
	if whole[0] != 120*time.Second {
		t.Fatalf("unmeasured break = %v, want four whole 30s ads", whole[0])
	}

	// Each ad opens on 1s of dead air and ends on 1s: it airs 28s from 1s in.
	for i := range catalog {
		catalog[i].AirStartMs, catalog[i].AirEndMs = 1_000, 29_000
	}
	trimmed, err := planner(t, catalog, store).PlanBreaks(context.Background(), "ch-1", pod.Selection{}, nil, fromStart, plans)
	if err != nil {
		t.Fatal(err)
	}
	if trimmed[0] != 112*time.Second {
		t.Fatalf("break after the windows were measured = %v, want four 28s windows", trimmed[0])
	}
	for _, b := range store.byKey {
		for _, e := range b.Pod.Entries {
			if e.Path != "" && (e.AirStartMs != 1_000 || e.DurationMs != 28_000) {
				t.Errorf("stored entry %s airs %dms from %dms, want 28000ms from 1000ms", e.Hash, e.DurationMs, e.AirStartMs)
			}
		}
	}
}
