package schedule_test

import (
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
)

// mapAvail is a pure Availability for tests: keys present in the map are
// available at the mapped library item id.
type mapAvail map[provision.Key]string

func (m mapAvail) Resolve(k provision.Key) (string, bool) {
	id, ok := m[k]
	return id, ok
}

func entry(key, title string) schedule.LineupEntry {
	return schedule.LineupEntry{Key: provision.Key(key), Title: title}
}

func seqChannel() schedule.Channel {
	return schedule.Channel{ID: "ch1", Name: "Test", Number: 5, Strategy: schedule.Sequential}
}

func TestComputeDesired_Sequential_AvailableBecomesProgram(t *testing.T) {
	entries := []schedule.LineupEntry{
		entry("movie:tmdb:1", "A"),
		entry("movie:tmdb:2", "B"),
	}
	avail := mapAvail{"movie:tmdb:1": "lib-A", "movie:tmdb:2": "lib-B"}

	got := schedule.ComputeDesired(seqChannel(), entries, avail, schedule.PodFill)

	if len(got.Slots) != 2 {
		t.Fatalf("want 2 slots, got %d", len(got.Slots))
	}
	if got.ProgramCount() != 2 {
		t.Fatalf("want 2 programs, got %d", got.ProgramCount())
	}
	// Order preserved for Sequential.
	if got.Slots[0].Title != "A" || got.Slots[1].Title != "B" {
		t.Fatalf("sequential order not preserved: %+v", got.Slots)
	}
	if got.Slots[0].LibraryItemID != "lib-A" {
		t.Fatalf("want lib-A, got %q", got.Slots[0].LibraryItemID)
	}
}

func TestComputeDesired_PendingPodFill_KeepsKeyAsFiller(t *testing.T) {
	entries := []schedule.LineupEntry{entry("movie:tmdb:9", "Coming")}
	got := schedule.ComputeDesired(seqChannel(), entries, mapAvail{}, schedule.PodFill)

	s := got.Slots[0]
	if s.Kind != schedule.SlotFiller {
		t.Fatalf("pod-fill: want SlotFiller, got %s", s.Kind)
	}
	if s.Key != "movie:tmdb:9" {
		t.Fatalf("pod-fill must preserve key for backfill, got %q", s.Key)
	}
	if s.LibraryItemID != "" {
		t.Fatalf("pending slot must have no library item, got %q", s.LibraryItemID)
	}
	if got.ProgramCount() != 0 {
		t.Fatalf("pending slot must not count as a program")
	}
}

func TestComputeDesired_PendingComingSoon_KeepsKeyAsPending(t *testing.T) {
	entries := []schedule.LineupEntry{entry("series:tvdb:7", "Later")}
	got := schedule.ComputeDesired(seqChannel(), entries, mapAvail{}, schedule.ComingSoon)

	s := got.Slots[0]
	if s.Kind != schedule.SlotPending {
		t.Fatalf("coming-soon: want SlotPending, got %s", s.Kind)
	}
	if s.Key != "series:tvdb:7" {
		t.Fatalf("coming-soon must preserve key, got %q", s.Key)
	}
}

func TestComputeDesired_Shuffle_DeterministicUnderSeed(t *testing.T) {
	entries := []schedule.LineupEntry{
		entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"),
		entry("movie:tmdb:3", "C"), entry("movie:tmdb:4", "D"),
		entry("movie:tmdb:5", "E"),
	}
	avail := mapAvail{
		"movie:tmdb:1": "1", "movie:tmdb:2": "2", "movie:tmdb:3": "3",
		"movie:tmdb:4": "4", "movie:tmdb:5": "5",
	}
	ch := schedule.Channel{ID: "ch", Name: "S", Number: 1, Strategy: schedule.Shuffle, Shuffle: schedule.ShuffleParams{Seed: 42}}

	first := schedule.ComputeDesired(ch, entries, avail, schedule.PodFill)
	second := schedule.ComputeDesired(ch, entries, avail, schedule.PodFill)

	// Same seed → identical order every reconcile (§9: guide doesn't scramble).
	for i := range first.Slots {
		if first.Slots[i].Title != second.Slots[i].Title {
			t.Fatalf("shuffle not deterministic at %d: %q vs %q", i, first.Slots[i].Title, second.Slots[i].Title)
		}
	}
	// A different seed should (very likely) reorder — guards against a no-op shuffle.
	other := ch
	other.Shuffle.Seed = 99
	diff := schedule.ComputeDesired(other, entries, avail, schedule.PodFill)
	same := true
	for i := range first.Slots {
		if first.Slots[i].Title != diff.Slots[i].Title {
			same = false
			break
		}
	}
	if same {
		t.Fatalf("different seeds produced identical order; shuffle likely a no-op")
	}
}

func TestPlaceAvailable_ReplacesInPlace_Idempotent(t *testing.T) {
	entries := []schedule.LineupEntry{
		entry("movie:tmdb:1", "A"), // available
		entry("movie:tmdb:2", "B"), // pending
	}
	avail := mapAvail{"movie:tmdb:1": "lib-A"}
	d := schedule.ComputeDesired(seqChannel(), entries, avail, schedule.PodFill)

	// B lands.
	d2, changed := d.PlaceAvailable("movie:tmdb:2", "lib-B", "B", 3600000)
	if !changed {
		t.Fatalf("expected a change when a pending title lands")
	}
	// Placed IN PLACE — index 1 stays B, index 0 untouched (§9 stable placement).
	if d2.Slots[1].Kind != schedule.SlotProgram || d2.Slots[1].LibraryItemID != "lib-B" {
		t.Fatalf("B not placed in its own slot: %+v", d2.Slots[1])
	}
	if d2.Slots[0].Title != "A" {
		t.Fatalf("placement reshuffled the channel; slot 0 = %q", d2.Slots[0].Title)
	}
	// Duplicate event → no-op (idempotent).
	d3, changed2 := d2.PlaceAvailable("movie:tmdb:2", "lib-B", "B", 3600000)
	if changed2 {
		t.Fatalf("duplicate available event must be a no-op")
	}
	_ = d3
}

func TestSubstitute_ReplacesHoldingSlot(t *testing.T) {
	entries := []schedule.LineupEntry{entry("movie:tmdb:5", "Gone")}
	d := schedule.ComputeDesired(seqChannel(), entries, mapAvail{}, schedule.ComingSoon)

	fallback := schedule.Slot{Kind: schedule.SlotFiller, Title: "bumper"}
	d2, changed := d.Substitute("movie:tmdb:5", fallback)
	if !changed {
		t.Fatalf("expected substitution")
	}
	if d2.Slots[0].Kind != schedule.SlotFiller || d2.Slots[0].Title != "bumper" {
		t.Fatalf("substitution didn't take: %+v", d2.Slots[0])
	}
}

func TestRevalidateAgainstLibrary_DemotesVanishedProgram(t *testing.T) {
	entries := []schedule.LineupEntry{
		entry("movie:tmdb:1", "Stays"),
		entry("movie:tmdb:2", "Vanishes"),
	}
	avail := mapAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}
	d := schedule.ComputeDesired(seqChannel(), entries, avail, schedule.PodFill)
	if d.ProgramCount() != 2 {
		t.Fatalf("setup: want 2 programs")
	}

	// The library loses movie 2 (deleted/re-id'd).
	shrunk := mapAvail{"movie:tmdb:1": "lib-1"}
	d2, drifted := d.RevalidateAgainstLibrary(shrunk, schedule.PodFill)
	if !drifted {
		t.Fatalf("expected drift when a scheduled program vanishes")
	}
	if d2.Slots[1].IsProgram() {
		t.Fatalf("vanished program must be demoted, still a program: %+v", d2.Slots[1])
	}
	if d2.Slots[1].Key != "movie:tmdb:2" {
		t.Fatalf("demoted slot must keep key for re-backfill, got %q", d2.Slots[1].Key)
	}
	if !d2.Slots[0].IsProgram() {
		t.Fatalf("present program must be untouched")
	}
}

func TestChannelValidate(t *testing.T) {
	cases := []struct {
		name    string
		ch      schedule.Channel
		wantErr bool
	}{
		{"ok", schedule.Channel{ID: "a", Name: "n", Number: 1, Strategy: schedule.Sequential}, false},
		{"no id", schedule.Channel{Name: "n", Number: 1, Strategy: schedule.Sequential}, true},
		{"no name", schedule.Channel{ID: "a", Number: 1, Strategy: schedule.Sequential}, true},
		{"bad number", schedule.Channel{ID: "a", Name: "n", Number: 0, Strategy: schedule.Sequential}, true},
		{"bad strategy", schedule.Channel{ID: "a", Name: "n", Number: 1, Strategy: "nope"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ch.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
