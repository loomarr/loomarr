package schedule

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// This file GENERALIZES the seeded property coverage already started in
// separation_internal_test.go (which pins blockMax=2, gap=0, syndication-only). It
// adds, as seeded plain-Go loops (not a framework):
//
//	(1) slotByPolicy is a PERMUTATION — multiset-equal to its input — for every
//	    ordering mode (sequential / shuffle / syndication), including the non-program
//	    (pending/filler/flex) slots it carries through;
//	(2) determinism under a fixed seed for every ordering mode;
//	(3) checkWrapSeparation returns empty for placeable pools generalized to
//	    blockMax > 2 AND a non-zero SeriesMinGap (the existing test only covers
//	    blockMax=2, gap=0);
//	(4) deckCovers holds for arbitrary pool sizes.
//
// All randomness seeds POOL generation; the placement itself is deterministic from
// its own seed, so any failure reproduces from the printed trial index.

// slotID is a total-ordering identity for a Slot, so we can compare two slot
// sequences as MULTISETS (permutation check) — every field that distinguishes one
// slot from another participates.
func slotID(s Slot) string {
	return fmt.Sprintf("%s|%s|%s|%d", s.Kind, s.Key, s.LibraryItemID, s.DurationMs)
}

// multiset counts slots by identity.
func multiset(slots []Slot) map[string]int {
	m := make(map[string]int, len(slots))
	for _, s := range slots {
		m[slotID(s)]++
	}
	return m
}

// assertPermutation fails t if out is not a reordering of in (no drops, no dups, no
// invented slots) — multiset equality on the full slot identity.
func assertPermutation(t *testing.T, ctx string, in, out []Slot) {
	t.Helper()
	if len(in) != len(out) {
		t.Fatalf("%s: length changed: in=%d out=%d", ctx, len(in), len(out))
	}
	mi, mo := multiset(in), multiset(out)
	if len(mi) != len(mo) {
		t.Fatalf("%s: distinct-slot count changed: in=%d out=%d", ctx, len(mi), len(mo))
	}
	for id, n := range mi {
		if mo[id] != n {
			t.Fatalf("%s: multiplicity of %q changed: in=%d out=%d", ctx, id, n, mo[id])
		}
	}
	for id, n := range mo {
		if mi[id] != n {
			t.Fatalf("%s: output invented slot %q (x%d) not in input", ctx, id, n)
		}
	}
}

// allOrderingModes are the three concrete modes slotByPolicy dispatches on. (The
// `default` branch is a copy-through like sequential; OrderInherit never reaches
// slotByPolicy — the policy resolves it first.)
var allOrderingModes = []OrderingMode{OrderSequential, OrderShuffle, OrderSyndication}

// genMixedPool builds a random slot pool: several distinct series with a few
// episodes each, PLUS a random tail of non-program slots (pending/filler/flex). The
// max per-series episode count is capped relative to the pool so the pool stays
// placeable under a modest blockMax (mirrors how the existing property test avoids
// the unsatisfiable single-series-dominates case). durMs is uniform so the caller
// can reason about gap satisfiability. Returns the pool.
func genMixedPool(gen *rand.Rand, durMs int64) []Slot {
	nSeries := 4 + gen.Intn(4) // 4–7 distinct series
	var pool []Slot
	for s := 0; s < nSeries; s++ {
		eps := 1 + gen.Intn(3) // 1–3 episodes → most-frequent series stays a small fraction
		seriesKey := "series:tvdb:" + string(rune('A'+s))
		for e := 0; e < eps; e++ {
			pool = append(pool, Slot{
				Kind:          SlotProgram,
				Key:           provision.Key(seriesKey),
				LibraryItemID: fmt.Sprintf("%s-ep%d", seriesKey, e),
				DurationMs:    durMs,
			})
		}
	}
	// A random tail of non-program slots. slotByPolicy must carry these through
	// unchanged in count (appended after the ordered programs), so the permutation
	// invariant has to account for them.
	nOther := gen.Intn(4) // 0–3
	otherKinds := []SlotKind{SlotPending, SlotFiller, SlotFlex}
	for i := 0; i < nOther; i++ {
		k := otherKinds[gen.Intn(len(otherKinds))]
		var key provision.Key
		if k == SlotPending {
			key = provision.Key(fmt.Sprintf("series:tvdb:pending%d", i))
		}
		pool = append(pool, Slot{
			Kind:          k,
			Key:           key,
			LibraryItemID: "",
			DurationMs:    int64(30000 + i*1000),
		})
	}
	return pool
}

// (1) slotByPolicy is a permutation of its input for EVERY ordering mode, including
// the non-program slots. Sequential preserves program order but still must not drop
// or duplicate; shuffle/syndication reorder but stay multiset-equal.
func TestSlotByPolicy_IsPermutation_AllModes_Property(t *testing.T) {
	for _, mode := range allOrderingModes {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			gen := rand.New(rand.NewSource(1))
			for trial := 0; trial < 300; trial++ {
				rp := ResolvedPolicy{
					Ordering: mode,
					Sep: ResolvedSeparation{
						BlockMax:        2 + gen.Intn(4), // 2–5
						SeriesMinGap:    time.Duration(gen.Intn(3)) * time.Minute,
						EpisodeNoRepeat: time.Hour,
					},
				}
				pool := genMixedPool(gen, 60000)
				in := append([]Slot(nil), pool...) // snapshot before (guards accidental input mutation)
				out := slotByPolicy(pool, rp, int64(trial))
				assertPermutation(t,
					fmt.Sprintf("mode=%s trial=%d", mode, trial), in, out)
			}
		})
	}
}

// slotByPolicy must also be a straight pass-through (still a permutation) when the
// pool has NO program slots — the early return path.
func TestSlotByPolicy_NoProgramSlots_PassThrough(t *testing.T) {
	for _, mode := range allOrderingModes {
		rp := ResolvedPolicy{Ordering: mode, Sep: ResolvedSeparation{BlockMax: 2}}
		in := []Slot{
			{Kind: SlotPending, Key: "series:tvdb:pending", DurationMs: 0},
			{Kind: SlotFiller, DurationMs: 30000},
			{Kind: SlotFlex, DurationMs: 15000},
		}
		out := slotByPolicy(append([]Slot(nil), in...), rp, 42)
		assertPermutation(t, "no-program mode="+string(mode), in, out)
	}
}

// (2) Determinism: same seed → byte-identical output, for EVERY ordering mode (the
// existing suite only pins syndication). Two independent calls on equal inputs must
// agree slot-for-slot.
func TestSlotByPolicy_Deterministic_AllModes_Property(t *testing.T) {
	for _, mode := range allOrderingModes {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			gen := rand.New(rand.NewSource(7))
			for trial := 0; trial < 200; trial++ {
				rp := ResolvedPolicy{
					Ordering: mode,
					Sep: ResolvedSeparation{
						BlockMax:        1 + gen.Intn(5), // 1–5
						SeriesMinGap:    time.Duration(gen.Intn(4)) * time.Minute,
						EpisodeNoRepeat: time.Hour,
					},
				}
				pool := genMixedPool(gen, 60000)
				seed := int64(trial * 13)
				a := slotByPolicy(append([]Slot(nil), pool...), rp, seed)
				b := slotByPolicy(append([]Slot(nil), pool...), rp, seed)
				if len(a) != len(b) {
					t.Fatalf("mode=%s trial=%d nondeterministic length: %d vs %d", mode, trial, len(a), len(b))
				}
				for i := range a {
					if slotID(a[i]) != slotID(b[i]) {
						t.Fatalf("mode=%s trial=%d nondeterministic at %d: %q vs %q",
							mode, trial, i, slotID(a[i]), slotID(b[i]))
					}
				}
			}
		})
	}
}

// genPlaceablePool builds a program-only pool whose most-frequent series is capped
// to a fraction of the pool, so a cycle placement under blockMax and seriesMinGap
// exists. All slots share durMs, so a seriesMinGap of `gapMs` needs about
// ceil(gapMs/durMs) OTHER slots between two occurrences of a series; capping the max
// per-series count well below (pool - maxPerSeries) keeps that reachable.
//
// nSeries and the per-series cap scale with blockMax: a larger blockMax tolerates a
// larger run, but for the gap to be satisfiable we still need plenty of distinct
// series. Returns the pool and its distinct-series count.
func genPlaceablePool(gen *rand.Rand, durMs int64) []Slot {
	nSeries := 5 + gen.Intn(4) // 5–8 distinct series → gap has room
	var pool []Slot
	for s := 0; s < nSeries; s++ {
		eps := 1 + gen.Intn(2) // 1–2 episodes: no series exceeds ~2/(≥5) of the pool
		seriesKey := "series:tvdb:" + string(rune('A'+s))
		for e := 0; e < eps; e++ {
			pool = append(pool, Slot{
				Kind:          SlotProgram,
				Key:           provision.Key(seriesKey),
				LibraryItemID: fmt.Sprintf("%s-ep%d", seriesKey, e),
				DurationMs:    durMs,
			})
		}
	}
	return pool
}

// (3a) checkWrapSeparation returns empty for placeable pools generalized to
// blockMax > 2 (gap disabled). This lifts the existing property test (which pins
// blockMax=2, gap=0) to higher block limits: the pure block/no-adjacency-run
// constraint holds across the seam for every blockMax in {3,4,5} on placeable pools.
// This branch of the placer is sound.
func TestSlotByPolicy_WrapSeparation_BlockGeneralized_Property(t *testing.T) {
	const durMs = int64(60000)
	for _, blockMax := range []int{3, 4, 5} {
		blockMax := blockMax
		t.Run(fmt.Sprintf("blockMax=%d", blockMax), func(t *testing.T) {
			rp := ResolvedPolicy{
				Ordering: OrderSyndication,
				Sep: ResolvedSeparation{
					BlockMax:        blockMax,
					SeriesMinGap:    0,
					EpisodeNoRepeat: time.Hour,
				},
			}
			gen := rand.New(rand.NewSource(int64(blockMax) * 101))
			for trial := 0; trial < 200; trial++ {
				pool := genPlaceablePool(gen, durMs)
				placed := slotByPolicy(pool, rp, int64(trial))
				if v := checkWrapSeparation(placed, rp); len(v) > 0 {
					t.Fatalf("blockMax=%d trial=%d (%d slots): %d wrap violations on a placeable pool: %+v",
						blockMax, trial, len(pool), len(v), v[0])
				}
			}
		})
	}
}

// hasValidCyclicArrangement is a brute-force ORACLE: it reports whether SOME
// permutation of the pool has zero wrap-separation violations under rp. Used only to
// tell "the pool is genuinely placeable" apart from "the pool is unsatisfiable" so
// the (3b) assertion below can only ever fire on a placer defect, never on an
// unsatisfiable pool. Exponential — callers must keep pools small (n ≤ 9).
func hasValidCyclicArrangement(pool []Slot, rp ResolvedPolicy) bool {
	n := len(pool)
	used := make([]bool, n)
	order := make([]Slot, 0, n)
	var rec func() bool
	rec = func() bool {
		if len(order) == n {
			return len(checkWrapSeparation(order, rp)) == 0
		}
		for i := 0; i < n; i++ {
			if used[i] {
				continue
			}
			used[i] = true
			order = append(order, pool[i])
			if rec() {
				return true
			}
			order = order[:len(order)-1]
			used[i] = false
		}
		return false
	}
	return rec()
}

// (3b) The full combined constraint — blockMax > 2 AND a non-zero SeriesMinGap —
// GATED BY THE BRUTE-FORCE ORACLE: for every generated pool for which a
// violation-free cyclic layout provably EXISTS, slotByPolicy must produce one. This
// is the honest statement of "checkWrapSeparation returns empty for placeable pools"
// once the gap constraint is switched on.
//
// !!! KNOWN-FAILING — reveals a real placer defect, NOT a test bug. !!!
// The greedy separationRepair/pickNext plus the bounded healSeam do NOT always find a
// valid arrangement even when one exists once blockMax and SeriesMinGap are BOTH
// active: the greedy can strand a series' last copy with no gap-satisfying slot left,
// producing an interior "X X" adjacency (gap 0 < SeriesMinGap) that healSeam — which
// only rotates to fix the last→first seam — cannot repair. Concrete instance
// (blockMax=3, gap=1min, uniform 60s slots): pool {A:2,B:2,C:1,D:1,E:2,F:1} places as
// "B E C F B D E A A" (violation gap@8), yet "A B E A B E C D F" is valid. Across
// {3,4,5}×200 trials ~50 pools that are provably solvable get a violating placement.
// The correct fix is in non-test source (a complete/backtracking placer or a
// gap-aware seam heal), NOT here; per the task this test asserts the CORRECT
// behavior and is expected to fail until the placer is fixed.
func TestSlotByPolicy_WrapSeparation_BlockAndGap_OracleGated_Property(t *testing.T) {
	const durMs = int64(60000)
	for _, blockMax := range []int{3, 4, 5} {
		blockMax := blockMax
		t.Run(fmt.Sprintf("blockMax=%d", blockMax), func(t *testing.T) {
			rp := ResolvedPolicy{
				Ordering: OrderSyndication,
				Sep: ResolvedSeparation{
					BlockMax:        blockMax,
					SeriesMinGap:    time.Minute, // one 60s program between repeats satisfies it
					EpisodeNoRepeat: time.Hour,
				},
			}
			gen := rand.New(rand.NewSource(int64(blockMax) * 101))
			for trial := 0; trial < 200; trial++ {
				pool := genPlaceablePool(gen, durMs)
				if len(pool) > 9 {
					continue // keep the exponential oracle cheap
				}
				if !hasValidCyclicArrangement(pool, rp) {
					continue // genuinely unsatisfiable → a ladder case, not our concern (§7)
				}
				placed := slotByPolicy(pool, rp, int64(trial))
				if v := checkWrapSeparation(placed, rp); len(v) > 0 {
					t.Fatalf("blockMax=%d trial=%d (%d slots): placer left %d wrap violation(s) on a PROVABLY-SOLVABLE pool: %+v",
						blockMax, trial, len(pool), len(v), v[0])
				}
			}
		})
	}
}

// (4) deckCovers holds for arbitrary pool sizes: a syndication deck plays every
// distinct program exactly once, for pools ranging from tiny to large, single-series
// and multi-series alike. This generalizes TestSyndicationDeck_EachOncePerDeck
// (fixed 8-episode single series) across sizes and shapes.
func TestSyndicationDeck_Covers_ArbitrarySizes_Property(t *testing.T) {
	gen := rand.New(rand.NewSource(3))
	for trial := 0; trial < 300; trial++ {
		rp := ResolvedPolicy{
			Ordering: OrderSyndication,
			Sep: ResolvedSeparation{
				BlockMax:        1 + gen.Intn(4),
				SeriesMinGap:    time.Duration(gen.Intn(2)) * time.Minute,
				EpisodeNoRepeat: time.Hour,
			},
		}
		// Pool size 1..~24 across 1..6 distinct series with unique item ids.
		nSeries := 1 + gen.Intn(6)
		var pool []Slot
		for s := 0; s < nSeries; s++ {
			eps := 1 + gen.Intn(4)
			seriesKey := "series:tvdb:" + string(rune('A'+s))
			for e := 0; e < eps; e++ {
				pool = append(pool, Slot{
					Kind:          SlotProgram,
					Key:           provision.Key(seriesKey),
					LibraryItemID: fmt.Sprintf("%s-ep%d", seriesKey, e),
					DurationMs:    int64(60000 + e*1000),
				})
			}
		}
		deck := syndicationDeck(pool, rp, int64(trial))
		if !deckCovers(deck) {
			t.Fatalf("trial=%d (%d slots, %d series): deck must play each distinct program exactly once",
				trial, len(pool), nSeries)
		}
		if len(deck) != len(pool) {
			t.Fatalf("trial=%d: deck dropped/duplicated: got %d want %d", trial, len(deck), len(pool))
		}
		// A deck is itself a permutation of the pool.
		assertPermutation(t, fmt.Sprintf("deck trial=%d", trial), pool, deck)
	}
}
