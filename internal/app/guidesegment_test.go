package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
)

// rotatingCycle mimics the real scheduler: it returns a DIFFERENT arrangement per rolling window,
// the way windowSlice advances its start by the window index (§6.5). Records the instants it was
// asked about so a test can assert the guide re-resolved at each boundary.
type rotatingCycle struct {
	mu      sync.Mutex
	askedAt []time.Time
	window  time.Duration
}

func (c *rotatingCycle) CyclePreview(_ context.Context, _ string, at time.Time) (
	time.Time, []schedule.Slot, schedule.ActiveRuleAttribution, time.Duration, error,
) {
	c.mu.Lock()
	c.askedAt = append(c.askedAt, at)
	c.mu.Unlock()

	// One programme per window, named for the window it belongs to — so the projected guide
	// spells out which arrangement each segment came from. An unbounded window (0) never
	// rotates, so every resolve yields the same single arrangement.
	title := "unbounded"
	dur := 2 * time.Hour
	if c.window > 0 {
		idx := at.Unix() / int64(c.window/time.Second)
		title = "win-" + time.Unix(idx*int64(c.window/time.Second), 0).UTC().Format("0102-15")
		dur = c.window
	}
	return at, []schedule.Slot{{
		Kind: schedule.SlotProgram, Key: provision.Key("movie:tmdb:1"),
		Title: title, LibraryItemID: "x", DurationMs: dur.Milliseconds(),
	}}, schedule.ActiveRuleAttribution{}, c.window, nil
}

func (c *rotatingCycle) asks() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.askedAt)
}

// THE DEFECT THIS CLOSES: the guide resolved ONE cycle at `from` and looped it across the whole
// requested span, so everything past the first rolling-window boundary forecast a rotation that
// will never happen. Reconciliation commits each new window for AiringNow; the guide must forecast
// the same later arrangements before they are committed.
//
// Measured on a live 27-title channel at the same instant 24h out: the guide advertised RoboCop /
// The Running Man / The Empire Strikes Back while the scheduler would arrange Lethal Weapon /
// Lethal Weapon 2 / Akira.
func TestSegmentedBroadcasts_ReResolvesAtEachWindowBoundary(t *testing.T) {
	eng := &rotatingCycle{window: 24 * time.Hour}
	r := &playoutResolver{engine: eng, now: time.Now}

	from := time.Now().Truncate(24 * time.Hour)
	to := from.Add(72 * time.Hour) // three windows

	bs, err := r.segmentedBroadcasts(context.Background(), "ch1", from, to, playout.BroadcastsBetween)
	if err != nil {
		t.Fatalf("segmentedBroadcasts: %v", err)
	}

	titles := map[string]bool{}
	for _, b := range bs {
		titles[b.Title] = true
	}
	if len(titles) < 3 {
		t.Fatalf("distinct arrangements across 3 windows = %d, want 3 — the guide is still looping one cycle (%v)", len(titles), titles)
	}
	if eng.asks() < 3 {
		t.Fatalf("CyclePreview asked %d times, want >= 3 (one per window boundary)", eng.asks())
	}
}

// An UNBOUNDED window (WindowFull) means the deck never rotates, so segmentation must collapse to
// exactly one resolve — the pre-existing behaviour, and the cheap path.
func TestSegmentedBroadcasts_UnboundedWindowResolvesOnce(t *testing.T) {
	eng := &rotatingCycle{window: 0}
	r := &playoutResolver{engine: eng, now: time.Now}

	from := time.Now().Truncate(time.Hour)
	if _, err := r.segmentedBroadcasts(context.Background(), "ch1", from, from.Add(72*time.Hour),
		playout.BroadcastsBetween); err != nil {
		t.Fatalf("segmentedBroadcasts: %v", err)
	}
	if got := eng.asks(); got != 1 {
		t.Fatalf("CyclePreview asked %d times for an unbounded window, want 1", got)
	}
}

// A pathological window must not turn one guide request into thousands of arrangements.
func TestSegmentedBroadcasts_IsBoundedBySegmentCap(t *testing.T) {
	eng := &rotatingCycle{window: time.Minute} // absurd: a one-minute rotation
	r := &playoutResolver{engine: eng, now: time.Now}

	from := time.Now().Truncate(time.Minute)
	if _, err := r.segmentedBroadcasts(context.Background(), "ch1", from, from.Add(24*time.Hour),
		playout.BroadcastsBetween); err != nil {
		t.Fatalf("segmentedBroadcasts: %v", err)
	}
	if got := eng.asks(); got > maxGuideSegments {
		t.Fatalf("CyclePreview asked %d times, want <= %d (the cap is not holding)", got, maxGuideSegments)
	}
}

// The guide's CURRENT block is observed broadcast state, not a forecast. Refreshing its forecast
// cache must therefore never replace the block a viewer is actually watching. Future rolling
// windows may still be previewed; the window containing now comes from the same persisted cycle as
// AiringNow.
func TestSegmentedBroadcasts_CurrentWindowUsesThePersistedAcceptedCycle(t *testing.T) {
	now := time.Date(2026, time.August, 15, 13, 30, 0, 0, time.UTC)
	eng := &rotatingCycle{window: 24 * time.Hour}
	accepted := &stubChannels{}
	accepted.ch.Desired = []schedule.Slot{{
		Kind: schedule.SlotProgram, Key: provision.Key("movie:tmdb:accepted"),
		Title: "accepted broadcast", LibraryItemID: "accepted", DurationMs: (24 * time.Hour).Milliseconds(),
	}}
	r := &playoutResolver{engine: eng, channels: accepted, now: func() time.Time { return now }}

	from := now.Add(-time.Hour)
	bs, err := r.segmentedBroadcasts(
		context.Background(), "ch1", from, now.Add(time.Hour), playout.BroadcastsBetween,
	)
	if err != nil {
		t.Fatalf("segmentedBroadcasts: %v", err)
	}
	if len(bs) == 0 || bs[0].Title != "accepted broadcast" {
		t.Fatalf("current guide = %+v, want the persisted accepted broadcast", bs)
	}
}
