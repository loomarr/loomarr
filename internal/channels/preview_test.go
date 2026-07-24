package channels_test

import (
	"context"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// CyclePreview resolves "what airs at `at`" through the SAME pure builder reconcile uses,
// attributes the active rule, and touches no Tunarr — the §8.1 time-travel preview.
func TestCyclePreview_PicksRuleAtChosenTimeAndIsReadOnly(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}
	e := newEngine(st, tun, avail, nil)

	// A channel with a weekend rule (higher priority) over the always-on base.
	weekend := schedule.SchedulingRule{ID: "w", Label: "Weekend Movies", Priority: 10, When: schedule.WhenPredicate{Weekend: true}}
	ch := store.Channel{Lineup: []schedule.LineupEntry{
		{Key: provision.Key("movie:tmdb:1"), Title: "A", DurationMs: 3600000},
		{Key: provision.Key("movie:tmdb:2"), Title: "B", DurationMs: 3600000},
	}}
	ch.ID = "c1"
	ch.Name = "Movies"
	ch.Number = 5
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusBuilding
	ch.Policy = schedule.ChannelPolicy{Rules: []schedule.SchedulingRule{weekend}}
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	sat := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC) // Saturday
	thu := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC) // Thursday

	// On Saturday the weekend rule is active and attributed.
	at, slots, active, _, err := e.CyclePreview(context.Background(), "c1", sat)
	if err != nil {
		t.Fatal(err)
	}
	if !at.Equal(sat) {
		t.Errorf("resolved at = %v, want %v", at, sat)
	}
	if !active.Matched || active.ID != "w" || active.Label != "Weekend Movies" {
		t.Errorf("Saturday attribution = %+v, want the weekend rule", active)
	}
	if len(slots) == 0 {
		t.Fatal("expected program slots on Saturday")
	}

	// On Thursday no rule matches → base policy.
	_, _, active2, _, err := e.CyclePreview(context.Background(), "c1", thu)
	if err != nil {
		t.Fatal(err)
	}
	if active2.Matched || active2.Label != "Base policy" {
		t.Errorf("Thursday attribution = %+v, want base policy", active2)
	}

	// Read-only: the preview must never touch Tunarr (no create/push/delete).
	if tun.Creates != 0 || tun.Pushes != 0 {
		t.Errorf("preview touched Tunarr: creates=%d pushes=%d (must be 0)", tun.Creates, tun.Pushes)
	}
}

// A zero `at` resolves to the engine's injected clock ("now"), echoed back so a caller
// always knows the moment it's looking at.
func TestCyclePreview_ZeroAtUsesNow(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	e := newEngine(st, tun, mapAvail{}, nil)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"))

	at, _, _, _, err := e.CyclePreview(context.Background(), "c1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// newEngine's clock is time.Unix(1_800_000_000, 0).UTC().
	if want := time.Unix(1_800_000_000, 0).UTC(); !at.Equal(want) {
		t.Errorf("zero at resolved to %v, want the engine clock %v", at, want)
	}
}
