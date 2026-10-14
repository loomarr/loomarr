package schedule

import (
	"math/rand"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
)

// progSlot builds a program slot for a series key with a given item id + duration.
func progSlot(seriesKey, itemID string, durMs int64) Slot {
	return Slot{Kind: SlotProgram, Key: provision.Key(seriesKey), LibraryItemID: itemID, DurationMs: durMs}
}

// --- §10 SEPARATION-WITH-WRAP (property test) -----------------------------------

// After separation-aware slotting, the placed cycle must have NO block/gap
// violation ACROSS THE SEAM (§3) whenever the pool is large enough to allow it. We
// generate many random multi-series pools (seeded, reproducible), place them, and
// assert checkWrapSeparation is empty. Pools with enough distinct series to satisfy
// blockMax=2 should always be placeable.
func TestSlotting_WrapSeparation_Property(t *testing.T) {
	rp := ResolvedPolicy{
		Ordering: OrderSyndication,
		Sep:      ResolvedSeparation{BlockMax: 2, SeriesMinGap: 0, EpisodeNoRepeat: time.Hour},
	}
	gen := rand.New(rand.NewSource(1)) // seeds the POOL generation, not the placement
	for trial := 0; trial < 200; trial++ {
		// 4–6 distinct series with balanced episode counts. blockMax=2 is satisfiable
		// on a cycle only when no single series exceeds ~half the pool, so we cap each
		// series' episodes so the most-frequent series never dominates (this test
		// asserts PLACEABLE pools get placed; an unsatisfiable pool is a ladder case,
		// tested separately). Each series gets 1–3 episodes across ≥4 series.
		nSeries := 4 + gen.Intn(3)
		var pool []Slot
		for s := 0; s < nSeries; s++ {
			eps := 1 + gen.Intn(3) // 1–3, so the max series ≤ 3 of ≥8 total → placeable
			seriesKey := "series:tvdb:" + string(rune('A'+s))
			for e := 0; e < eps; e++ {
				pool = append(pool, progSlot(seriesKey, seriesKey+"-"+string(rune('0'+e)), int64(60000+e*1000)))
			}
		}
		placed := slotByPolicy(pool, rp, int64(trial))
		if v := checkWrapSeparation(placed, rp); len(v) > 0 {
			t.Fatalf("trial %d (%d slots): %d wrap-separation violations for a placeable pool: %+v",
				trial, len(pool), len(v), v[0])
		}
	}
}

// The seam heal specifically fixes the last→first adjacency: a sequence that ends
// and starts with the same series (a seam block) is rotated so the seam is clean.
func TestSlotting_HealsCycleSeam(t *testing.T) {
	// blockMax=1 makes any same-series adjacency (interior OR seam) a violation. A
	// balanced 3-A/3-B pool CAN be laid out ABABAB with a clean seam, so the placer +
	// seam heal must find it. This isolates the seam behavior: a naive shuffle would
	// often leave the last and first slots the same series.
	rp := ResolvedPolicy{Ordering: OrderSyndication, Sep: ResolvedSeparation{BlockMax: 1}}
	pool := []Slot{
		progSlot("series:tvdb:A", "a1", 60000),
		progSlot("series:tvdb:A", "a2", 60000),
		progSlot("series:tvdb:A", "a3", 60000),
		progSlot("series:tvdb:B", "b1", 60000),
		progSlot("series:tvdb:B", "b2", 60000),
		progSlot("series:tvdb:B", "b3", 60000),
	}
	placed := slotByPolicy(pool, rp, 7)
	if v := checkWrapSeparation(placed, rp); len(v) > 0 {
		t.Errorf("seam/interior not clean for a placeable ABABAB pool: %+v", v)
	}
}

// --- §10 SYNDICATION DECK -------------------------------------------------------

// A syndication deck plays every distinct program exactly once before any repeat
// (§5) — the deck IS a no-repeat structure.
func TestSyndicationDeck_EachOncePerDeck(t *testing.T) {
	rp := ResolvedPolicy{Ordering: OrderSyndication, Sep: ResolvedSeparation{BlockMax: 2}}
	var pool []Slot
	for i := 0; i < 8; i++ {
		pool = append(pool, progSlot("series:tvdb:S", "ep-"+string(rune('0'+i)), 60000))
	}
	deck := syndicationDeck(pool, rp, 99)
	if !deckCovers(deck) {
		t.Error("a deck must play each distinct program exactly once before any repeat")
	}
	if len(deck) != len(pool) {
		t.Errorf("deck dropped or duplicated slots: got %d want %d", len(deck), len(pool))
	}
}

// Successive decks differ (seed XOR deckIndex) yet each is deterministic for its
// seed — the reproducibility property (§5/§7).
func TestSyndicationDeck_DeterministicPerSeed(t *testing.T) {
	rp := ResolvedPolicy{Ordering: OrderSyndication, Sep: ResolvedSeparation{BlockMax: 4}}
	var pool []Slot
	for i := 0; i < 6; i++ {
		pool = append(pool, progSlot("series:tvdb:S", "ep-"+string(rune('0'+i)), 60000))
	}
	d1 := syndicationDeck(pool, rp, 5)
	d2 := syndicationDeck(pool, rp, 5)
	for i := range d1 {
		if d1[i].LibraryItemID != d2[i].LibraryItemID {
			t.Fatalf("same seed gave different decks at %d: %q vs %q", i, d1[i].LibraryItemID, d2[i].LibraryItemID)
		}
	}
}
