package channels_test

import (
	"context"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/channels"
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
	ch.Policy = schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Rules: []schedule.SchedulingRule{weekend}}}
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

func TestCyclePreview_ReadsLiveChannelDefaults(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}
	window := 24 * time.Hour
	breaks := 0
	e := channels.New(st, tun, avail, nil, channels.Config{
		ResolveDefaultWindow: func() time.Duration { return window },
		ResolveBreaksPerHour: func() int { return breaks },
	}, func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, testkit.Logger()).
		WithPods(&fakePods{ids: []string{"clip-a"}})
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))

	_, slots, _, gotWindow, err := e.CyclePreview(context.Background(), "c1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if gotWindow != 24*time.Hour {
		t.Fatalf("initial window = %v, want 24h", gotWindow)
	}
	if got := fillerSlotCount(slots); got != 0 {
		t.Fatalf("initial filler slots = %d, want none", got)
	}

	window = 48 * time.Hour
	breaks = 30
	_, slots, _, gotWindow, err = e.CyclePreview(context.Background(), "c1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if gotWindow != 48*time.Hour {
		t.Fatalf("window after settings change = %v, want 48h", gotWindow)
	}
	if got := fillerSlotCount(slots); got == 0 {
		t.Fatal("break frequency change did not affect the next preview")
	}
}

func fillerSlotCount(slots []schedule.Slot) int {
	n := 0
	for _, slot := range slots {
		if slot.Kind == schedule.SlotFiller {
			n++
		}
	}
	return n
}

// The exclusion report reaches the caller (#263). ComputeDesiredAt has always produced it and
// every caller discarded it — reconcile still does — so this is the ONE path by which "why isn't
// X on my channel" is answerable at all. The API-level test drives a fake engine, so only this
// one proves the real engine carries the report rather than a zero value.
func TestCyclePreviewDraft_CarriesTheExclusionReport(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}
	e := newEngine(st, tun, avail, nil)

	// One admissible title and one the ceiling must refuse.
	ch := store.Channel{Lineup: []schedule.LineupEntry{
		{Key: provision.Key("movie:tmdb:1"), Title: "Kids Film", OfficialRating: "TV-Y7", DurationMs: 3600000},
		{Key: provision.Key("movie:tmdb:2"), Title: "Adult Film", OfficialRating: "TV-MA", DurationMs: 3600000},
	}}
	ch.ID = "c1"
	ch.Name = "Kids"
	ch.Number = 5
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusBuilding
	ch.Policy = schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Audience: schedule.AudiencePolicy{Ceiling: "TV-Y7"},
	}}
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	got, err := e.CyclePreviewDraft(context.Background(), "c1", time.Time{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Excluded.OverCeiling != 1 {
		t.Errorf("overCeiling = %d, want 1 — the TV-MA title must be reported as refused, not "+
			"merely absent from the slots", got.Excluded.OverCeiling)
	}
	// Absent-from-slots is NOT the same claim: a title can vanish for a dozen reasons (window,
	// rule narrowing, seasonal bench). The report is what says WHY, so it must name the item.
	if len(got.Excluded.Items) != 1 || got.Excluded.Items[0].Title != "Adult Film" ||
		got.Excluded.Items[0].Reason != "over_ceiling" {
		t.Errorf("items = %+v, want one over_ceiling item naming Adult Film", got.Excluded.Items)
	}
	// …and the admissible one still airs, or "nothing was excluded" would be trivially true.
	if len(got.Slots) == 0 {
		t.Fatal("expected the below-ceiling title to still air")
	}
}
