package schedule

import (
	"github.com/mantonx/loomarr/internal/provision"
)

// This file holds the separation-aware slot placement (programming-design §3, §5):
// the syndication "deck deal", the block/gap constraint engine, and the cycle-seam
// wrap check. All placement is seeded-deterministic.

// syndicationDeck deals the eligible programs like a shuffled deck (§5): each
// program airs once per deck before any repeat, then the deck reshuffles under
// `seed XOR deckIndex` so successive decks differ yet each is reproducible. This
// makes episodeNoRepeat nearly free (the deck IS a no-repeat structure) and gives
// the authentic weekday-rerun texture.
//
// Within a deck the order is separation-repaired (blockMax / seriesMinGap) so a
// single deck doesn't stack a series back-to-back-to-back. We emit ONE deck here —
// Tunarr loops the resulting cycle, and successive loops are effectively
// deck-then-deck; the seam heal covers the deck→deck boundary.
func syndicationDeck(programs []Slot, rp ResolvedPolicy, seed int64) []Slot {
	deck := seededShuffle(programs, seed)
	return separationRepair(deck, rp)
}

// separationRepair reorders a program sequence to satisfy the block/gap
// constraints (§3) as far as the pool allows. It never drops a slot — if a
// constraint can't be satisfied (pool too small), it places the least-bad candidate
// and moves on (the relaxation ladder, §7, records that the window couldn't be met).
//
// Placement is MOST-CONSTRAINED-FIRST: among the candidates that don't break the
// trailing run/gap, it picks the one whose SERIES has the most remaining slots —
// spending down the scarce (most-frequent) series first. A plain earliest-acceptable
// greedy defers the frequent series and then has to stack it at the end; picking the
// most-remaining series provably avoids that whenever a valid arrangement exists.
// Deterministic: ties break by the incoming order, no randomness of its own.
func separationRepair(programs []Slot, rp ResolvedPolicy) []Slot {
	n := len(programs)
	if n <= 1 {
		return programs
	}
	remaining := append([]Slot(nil), programs...)
	out := make([]Slot, 0, n)

	for len(remaining) > 0 {
		idx := pickNext(out, remaining, rp)
		out = append(out, remaining[idx])
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}
	return out
}

// remainingCounts tallies how many slots per series key are left to place.
func remainingCounts(remaining []Slot) map[provision.Key]int {
	c := make(map[provision.Key]int, len(remaining))
	for _, s := range remaining {
		c[seriesKeyOf(s)]++
	}
	return c
}

// pickNext chooses the index in `remaining` to place next, most-constrained-first
// with anti-clustering. Preference order among candidates:
//  1. honors separation (trailing run < blockMax, seriesMinGap satisfied) AND is a
//     DIFFERENT series than the immediately-preceding slot — spreads the frequent
//     series out (avoids "A A ..." when an A-then-other placement is available);
//  2. honors separation (may continue the same series, up to blockMax);
//  3. anything (pool exhausted for the constraint) — least-bad, never a drop.
//
// Within each tier the candidate whose series has the MOST remaining slots wins
// (spend the scarce/frequent series down first), ties by incoming order → deterministic.
func pickNext(out, remaining []Slot, rp ResolvedPolicy) int {
	counts := remainingCounts(remaining)
	var prevKey provision.Key
	if len(out) > 0 {
		prevKey = seriesKeyOf(out[len(out)-1])
	}

	bestSwitch, bestSwitchRem := -1, -1 // tier 1: OK + different series
	bestOK, bestOKRem := -1, -1         // tier 2: OK (same series allowed)
	bestAny, bestAnyRem := -1, -1       // tier 3: fallback

	for i, cand := range remaining {
		key := seriesKeyOf(cand)
		rem := counts[key]
		if rem > bestAnyRem {
			bestAny, bestAnyRem = i, rem
		}
		if !separationOK(out, cand, rp) {
			continue
		}
		if rem > bestOKRem {
			bestOK, bestOKRem = i, rem
		}
		if key != prevKey && rem > bestSwitchRem {
			bestSwitch, bestSwitchRem = i, rem
		}
	}
	switch {
	case bestSwitch >= 0:
		return bestSwitch
	case bestOK >= 0:
		return bestOK
	default:
		return bestAny
	}
}

// separationOK reports whether placing `cand` right after `out` honors blockMax and
// seriesMinGap for cand's series (§3).
func separationOK(out []Slot, cand Slot, rp ResolvedPolicy) bool {
	key := seriesKeyOf(cand)

	// blockMax: count the trailing run of the same series in `out`.
	if rp.Sep.BlockMax > 0 {
		run := 0
		for i := len(out) - 1; i >= 0; i-- {
			if seriesKeyOf(out[i]) == key {
				run++
			} else {
				break
			}
		}
		if run >= rp.Sep.BlockMax {
			return false
		}
	}

	// seriesMinGap: accumulated runtime since the last slot of this series must be
	// at least the gap. Only meaningful on a mixed channel (single-series relaxes it
	// to 0 in Resolved, so this check is skipped there).
	if rp.Sep.SeriesMinGap > 0 {
		var sinceMs int64 = -1 // -1 = series not seen in out
		var accMs int64
		for i := len(out) - 1; i >= 0; i-- {
			if seriesKeyOf(out[i]) == key {
				sinceMs = accMs
				break
			}
			accMs += out[i].DurationMs
		}
		if sinceMs >= 0 && sinceMs < rp.Sep.SeriesMinGap.Milliseconds() {
			return false
		}
	}
	return true
}

// healSeam fixes the cycle-seam (§3): because Tunarr loops the list, the last
// slot is adjacent to the first, so the seam must honor blockMax/seriesMinGap too.
// If the seam violates (same series wraps around), rotate the sequence to a start
// point that breaks the run — a bounded, order-preserving fix. If no rotation heals
// it (a single-series channel, or a pool that can't avoid it), the sequence is left
// as-is (the relaxation ladder handles a genuinely unsatisfiable window).
func healSeam(programs []Slot, rp ResolvedPolicy) []Slot {
	n := len(programs)
	if n <= 1 || !seamViolates(programs, rp) {
		return programs
	}
	// Try each rotation; take the first that makes the seam clean while keeping the
	// interior order. Deterministic (smallest rotation offset wins).
	for off := 1; off < n; off++ {
		rot := rotate(programs, off)
		if !seamViolates(rot, rp) {
			return rot
		}
	}
	return programs // unhealable within the pool → leave it (ladder's problem)
}

// seamViolates reports whether the last→first wrap breaks separation (§3): the same
// series straddling the seam beyond blockMax, or within seriesMinGap.
func seamViolates(programs []Slot, rp ResolvedPolicy) bool {
	n := len(programs)
	if n <= 1 {
		return false
	}
	firstKey := seriesKeyOf(programs[0])

	// blockMax across the seam: trailing run at the end + leading run at the start,
	// if they're the same series, must not exceed blockMax.
	if rp.Sep.BlockMax > 0 {
		tail := 0
		for i := n - 1; i >= 0 && seriesKeyOf(programs[i]) == firstKey; i-- {
			tail++
		}
		lead := 0
		for i := 0; i < n && seriesKeyOf(programs[i]) == firstKey; i++ {
			lead++
		}
		// tail includes slots that are the same series as the FIRST; if the last slot
		// isn't the first's series, tail is 0 and there's no seam block.
		if seriesKeyOf(programs[n-1]) == firstKey && tail+lead > rp.Sep.BlockMax {
			return true
		}
	}

	// seriesMinGap across the seam: runtime from the last occurrence of firstKey at
	// the tail, wrapping to programs[0], must meet the gap. Simplest sufficient
	// check: if the last slot is the same series as the first, the gap is 0 → violate.
	if rp.Sep.SeriesMinGap > 0 && seriesKeyOf(programs[n-1]) == firstKey {
		return true
	}
	return false
}

// rotate returns programs rotated left by off (order-preserving on the cycle).
func rotate(programs []Slot, off int) []Slot {
	n := len(programs)
	out := make([]Slot, 0, n)
	out = append(out, programs[off:]...)
	out = append(out, programs[:off]...)
	return out
}

// --- wrap-aware separation validation (used by the property test, §10) ----------

// Violation is one separation breach found by checkWrapSeparation.
type Violation struct {
	Kind  string        // "block" | "gap" | "episode_repeat"
	Key   provision.Key // the series/title involved
	Index int           // slot index where the breach begins
}

// checkWrapSeparation validates a placed program sequence against the resolved
// separation rules ACROSS THE CYCLE SEAM (§3): it treats the sequence as a loop
// (index n-1 adjacent to 0) and reports every blockMax / seriesMinGap breach. The
// property test asserts this returns empty for a well-placed cycle. It only checks
// the texture rules (block/gap); the no-repeat window is a deck property tested
// separately.
func checkWrapSeparation(programs []Slot, rp ResolvedPolicy) []Violation {
	n := len(programs)
	if n <= 1 {
		return nil
	}
	var v []Violation

	// blockMax: scan consecutive runs on the wrapped sequence (2n walk covers the seam).
	if rp.Sep.BlockMax > 0 {
		run := 1
		for i := 1; i < 2*n; i++ {
			cur := seriesKeyOf(programs[i%n])
			prev := seriesKeyOf(programs[(i-1)%n])
			if cur == prev {
				run++
				if run > rp.Sep.BlockMax {
					v = append(v, Violation{Kind: "block", Key: cur, Index: i % n})
				}
			} else {
				run = 1
			}
			if i >= n && run == 1 {
				break // wrapped past the seam and the run reset → done
			}
		}
	}

	// seriesMinGap: for each slot, walk backward on the cycle accumulating runtime
	// until the same series recurs; a gap under the minimum is a violation.
	if rp.Sep.SeriesMinGap > 0 {
		gapMs := rp.Sep.SeriesMinGap.Milliseconds()
		for i := 0; i < n; i++ {
			key := seriesKeyOf(programs[i])
			var accMs int64
			for step := 1; step < n; step++ {
				j := ((i-step)%n + n) % n
				if seriesKeyOf(programs[j]) == key {
					if accMs < gapMs {
						v = append(v, Violation{Kind: "gap", Key: key, Index: i})
					}
					break
				}
				accMs += programs[j].DurationMs
			}
		}
	}
	return v
}

// deckCovers reports whether a single deck plays every distinct program exactly
// once before any repeat — the syndication-deck invariant (§5). Used by tests.
func deckCovers(programs []Slot) bool {
	seen := map[string]int{}
	for _, s := range programs {
		seen[deckID(s)]++
	}
	for _, c := range seen {
		if c != 1 {
			return false
		}
	}
	return true
}

// deckID identifies a program within a deck: its library item (an episode is its
// own item; a movie its item). Falls back to the Key for a program without an item.
func deckID(s Slot) string {
	if s.LibraryItemID != "" {
		return s.LibraryItemID
	}
	return string(s.Key)
}
