package schedule

import (
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

func recSlot(id, title string, dur int64) Slot {
	return Slot{Kind: SlotProgram, Key: provision.Key("movie:tmdb:" + id), Title: title, DurationMs: dur}
}

// THE GAP THIS CLOSES: separation (§3) is a WITHIN-CYCLE rule — when the deck wraps, the
// scheduler's memory resets and titles recur on a positional clock rather than a programmed one.
// Reported live: Akira aired Tue 21:53, Fri 13:33, Sat 02:10 and Mon 01:30, four times in a week
// at no interval anyone chose.
func TestByRecency_LeastRecentlyAiredComesFirst(t *testing.T) {
	now := time.Now()
	slots := []Slot{
		recSlot("1", "Aired Yesterday", 1000),
		recSlot("2", "Aired A Week Ago", 1000),
		recSlot("3", "Aired An Hour Ago", 1000),
	}
	hist := map[provision.Key]time.Time{
		"movie:tmdb:1": now.Add(-24 * time.Hour),
		"movie:tmdb:2": now.Add(-168 * time.Hour),
		"movie:tmdb:3": now.Add(-time.Hour),
	}

	got := byRecency(slots, hist)
	want := []string{"Aired A Week Ago", "Aired Yesterday", "Aired An Hour Ago"}
	for i, w := range want {
		if got[i].Title != w {
			t.Fatalf("position %d = %q, want %q (order: %v)", i, got[i].Title, w, titlesOf(got))
		}
	}
}

// A title with NO history sorts first: never-aired is maximally "not recent", which is what puts
// a freshly-acquired title on screen promptly rather than behind everything already in rotation.
func TestByRecency_NeverAiredSortsFirst(t *testing.T) {
	now := time.Now()
	slots := []Slot{
		recSlot("1", "Aired Recently", 1000),
		recSlot("2", "Brand New", 1000),
	}
	hist := map[provision.Key]time.Time{"movie:tmdb:1": now.Add(-time.Hour)}

	got := byRecency(slots, hist)
	if got[0].Title != "Brand New" {
		t.Fatalf("never-aired should lead; got %v", titlesOf(got))
	}
}

// DETERMINISM (§3): the sort is STABLE over the already-seeded order, so equal-recency slots keep
// their shuffled positions. Same pool + policy + seed + history ⇒ byte-identical deck, which is
// what makes reconciles idempotent and tests reproducible.
func TestByRecency_IsStableForEqualRecency(t *testing.T) {
	slots := []Slot{
		recSlot("1", "A", 1000), recSlot("2", "B", 1000), recSlot("3", "C", 1000),
	}
	same := time.Now().Add(-time.Hour)
	hist := map[provision.Key]time.Time{
		"movie:tmdb:1": same, "movie:tmdb:2": same, "movie:tmdb:3": same,
	}
	for range 5 {
		got := byRecency(slots, hist)
		if got[0].Title != "A" || got[1].Title != "B" || got[2].Title != "C" {
			t.Fatalf("equal recency must preserve the seeded order; got %v", titlesOf(got))
		}
	}
}

// No history ⇒ the order is untouched. A fresh install, a channel that has never aired, or a
// store that could not answer must all degrade to exactly the pre-§3.1 behaviour.
func TestByRecency_NoHistoryLeavesOrderUntouched(t *testing.T) {
	slots := []Slot{recSlot("1", "A", 1000), recSlot("2", "B", 1000)}
	got := byRecency(slots, nil)
	if got[0].Title != "A" || got[1].Title != "B" {
		t.Fatalf("empty history must not reorder; got %v", titlesOf(got))
	}
}

// SEQUENTIAL is exempt: that mode means "play this show in episode order", and re-ordering it by
// recency would destroy the single property it exists to provide. Asserted at slotByPolicy, the
// function that makes the ordering choice.
func TestSlotByPolicy_SequentialIgnoresRecency(t *testing.T) {
	now := time.Now()
	slots := []Slot{
		recSlot("1", "First", 1000), recSlot("2", "Second", 1000), recSlot("3", "Third", 1000),
	}
	rp := ResolvedPolicy{
		Ordering: OrderSequential,
		LastAired: map[provision.Key]time.Time{
			"movie:tmdb:1": now.Add(-time.Hour),       // most recent — recency would sort it LAST
			"movie:tmdb:3": now.Add(-500 * time.Hour), // oldest — recency would sort it FIRST
		},
	}

	got := slotByPolicy(slots, rp, 42)
	var order []string
	for _, s := range got {
		if s.IsProgram() {
			order = append(order, s.Title)
		}
	}
	if len(order) != 3 || order[0] != "First" || order[2] != "Third" {
		t.Fatalf("sequential order must survive the recency signal; got %v", order)
	}
}

// SHUFFLE does consume the signal — the counterpart to the exemption above, so the test pair
// shows the split is deliberate rather than accidental.
func TestSlotByPolicy_ShuffleConsumesRecency(t *testing.T) {
	now := time.Now()
	slots := []Slot{
		recSlot("1", "Just Aired", 1000), recSlot("2", "Long Ago", 1000),
	}
	rp := ResolvedPolicy{
		Ordering: OrderShuffle,
		LastAired: map[provision.Key]time.Time{
			"movie:tmdb:1": now.Add(-time.Hour),
			"movie:tmdb:2": now.Add(-500 * time.Hour),
		},
	}

	got := slotByPolicy(slots, rp, 42)
	var first string
	for _, s := range got {
		if s.IsProgram() {
			first = s.Title
			break
		}
	}
	if first != "Long Ago" {
		t.Fatalf("shuffle should lead with the least-recently-aired; got %q", first)
	}
}

func titlesOf(slots []Slot) []string {
	out := make([]string, 0, len(slots))
	for _, s := range slots {
		out = append(out, s.Title)
	}
	return out
}
