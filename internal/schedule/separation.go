package schedule

import (
	"slices"
	"sort"

	"github.com/loomarr/loomarr/internal/provision"
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
// constraints across the CYCLE (§3, §8 "greedy with backtracking"). It never drops
// a slot; when a genuinely-unsatisfiable pool can't be arranged (too small ∩ too
// constrained — a §7 relaxation-ladder case), it returns the greedy least-bad order.
//
// It is GREEDY-ORDERED BACKTRACKING: at each position it tries candidates in the
// most-constrained-first / anti-clustering order (pickOrder) so the FIRST path is
// usually the answer, but it BACKTRACKS when a placement dead-ends — a plain
// single-pass greedy strands a series' last copy with no gap-satisfying slot left
// (the defect the oracle-gated property test caught: blockMax>2 ∧ SeriesMinGap>0).
// The search is cycle-aware: it prunes on interior violations as it builds AND
// validates the last→first seam on completion, so a valid result needs no separate
// healSeam. Bounded by backtrackBudget so a pathological pool can't blow up — on
// exhaustion it falls back to the greedy order (the ladder handles the shortfall).
func separationRepair(programs []Slot, rp ResolvedPolicy) []Slot {
	n := len(programs)
	if n <= 1 {
		return programs
	}
	// No texture constraints (both relaxed to 0, e.g. single-series) → nothing to
	// repair; keep the deck order.
	if rp.Sep.BlockMax <= 0 && rp.Sep.SeriesMinGap <= 0 {
		return programs
	}

	if solved, ok := backtrackArrange(programs, rp); ok {
		return solved
	}
	// Unsatisfiable within the budget → greedy least-bad (ladder records the miss).
	return greedyArrange(programs, rp)
}

// backtrackBudget bounds the number of placement attempts so a pathological pool
// can't make the search exponential. Envelope-scale cycles (tens of slots) resolve
// in far fewer; beyond the budget we fall back to greedy + let the ladder relax.
const backtrackBudget = 50_000

// backtrackArrange searches for a full cyclic arrangement with zero interior/seam
// separation violations, trying candidates in greedy-heuristic order at each step.
// Returns (arrangement, true) on success, (nil, false) if the budget is exhausted
// or no valid arrangement exists.
func backtrackArrange(programs []Slot, rp ResolvedPolicy) ([]Slot, bool) {
	n := len(programs)
	used := make([]bool, n)
	out := make([]Slot, 0, n)
	budget := backtrackBudget

	var rec func() bool
	rec = func() bool {
		if len(out) == n {
			// Full arrangement — accept iff the cycle (incl. the last→first seam) is clean.
			return len(checkWrapSeparation(out, rp)) == 0
		}
		order := newCandidateOrder(out, programs, used, rp)

		// Try remaining candidates in greedy priority order (most-constrained-first,
		// anti-clustering) so the first descent usually succeeds.
		//
		// LAZILY: `order` yields the best candidate from an O(n) scan and only pays for a
		// full sort of the rest if this loop actually asks for a second one. Measured on
		// every SOLVABLE deck, it never does — the first candidate wins at every position
		// (tried == number of positions, zero backtracks), so the old code sorted n
		// candidates at each of n positions to consume exactly one of them each time.
		//
		// The unsolvable path is deliberately UNCHANGED: it still ranks every candidate and
		// still spends budget on the separation-violating ones, which is what makes a hard
		// pool exhaust backtrackBudget and fall back to greedyArrange promptly. Removing
		// that was the 17× regression recorded on pickOrder.
		for idx, ok := order.next(); ok; idx, ok = order.next() {
			if budget <= 0 {
				return false // out of budget — abandon (caller falls back to greedy)
			}
			budget--
			// Prune: only extend with a candidate that keeps the PARTIAL sequence
			// interior-clean (blockMax run + gap since the last same-series slot).
			if !separationOK(out, programs[idx], rp) {
				continue
			}
			used[idx] = true
			out = append(out, programs[idx])
			if rec() {
				return true
			}
			out = out[:len(out)-1]
			used[idx] = false
		}
		return false
	}
	if rec() {
		return append([]Slot(nil), out...), true
	}
	return nil, false
}

// greedyArrange is the single-pass most-constrained-first placement used as the
// fallback when backtracking can't satisfy the pool (a genuine ladder case). It
// never drops a slot — the least-bad candidate is placed when none fully fits.
func greedyArrange(programs []Slot, rp ResolvedPolicy) []Slot {
	n := len(programs)
	remaining := append([]Slot(nil), programs...)
	out := make([]Slot, 0, n)
	for len(remaining) > 0 {
		idx := pickNext(out, remaining, rp)
		out = append(out, remaining[idx])
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}
	return out
}

// cand is one placement candidate, ranked most-constrained-first with anti-clustering —
// the same ranking pickNext applies, expressed as sortable fields.
type cand struct {
	idx      int
	tier     int // 0 = OK+switch series, 1 = OK same series, 2 = not OK (fallback)
	rem      int // remaining count of this candidate's series (most-first)
	posOrder int // stable tiebreak
}

// candLess is THE ordering: lower tier first, then higher remaining count, then position.
// One definition, used by both the lazy first pick and the full sort, so the two can never
// disagree about which candidate is best.
func candLess(a, b cand) bool {
	if a.tier != b.tier {
		return a.tier < b.tier
	}
	if a.rem != b.rem {
		return a.rem > b.rem // most-remaining first
	}
	return a.posOrder < b.posOrder
}

// candidateOrder yields placement candidates in greedy priority order, LAZILY.
//
// Why lazily. Profiling put the old eager version at 55% of the guide's CPU, its sort alone
// at 34%: it fully sorted every remaining candidate at every position. Instrumenting the
// search showed why that is waste — on every SOLVABLE deck the first candidate wins at every
// position (tried == positions, zero backtracks), so it sorted n candidates n times to consume
// exactly one of them each time. The first `next()` is now an O(n) scan for the minimum; the
// sort happens only if the caller comes back, which the measurements say it does not on the
// path that matters.
//
// ⚠ What must NOT change, and why (this is a re-entry point for a mistake already made once).
// Tier-2 candidates — the ones that FAIL separation — are still produced and still handed to
// the caller, which skips them with `continue` but only AFTER spending budget. That budget
// drain is what makes a hard pool exhaust backtrackBudget and fall back to greedyArrange
// promptly. Filtering them out looks obviously correct and took GET /v1/guide from ~250ms to
// ~4.3s (a 17× regression) with every existing test green, because it removed that accidental
// circuit-breaker. Likewise the caller's own separationOK re-check is not redundant with the
// tier assignment here.
//
// Verify any change to this against a RUNNING SERVER. The benchmarks in
// compute_scale_bench_test.go do not reach this path — their decks solve on the first descent
// — which is exactly how that regression looked fine until it was measured live.
type candidateOrder struct {
	cands  []cand
	sorted bool
	i      int
}

// newCandidateOrder builds the (unsorted) candidate set for the current partial arrangement.
// The scan itself is unavoidable: every candidate's tier needs its separationOK, and `rem`
// needs the per-series remaining counts.
func newCandidateOrder(out, programs []Slot, used []bool, rp ResolvedPolicy) *candidateOrder {
	counts := map[provision.Key]int{}
	for i, s := range programs {
		if !used[i] {
			counts[seriesKeyOf(s)]++
		}
	}
	var prevKey provision.Key
	if len(out) > 0 {
		prevKey = seriesKeyOf(out[len(out)-1])
	}

	cands := make([]cand, 0, len(programs))
	for i, s := range programs {
		if used[i] {
			continue
		}
		key := seriesKeyOf(s)
		tier := 2
		if separationOK(out, s, rp) {
			if key != prevKey {
				tier = 0
			} else {
				tier = 1
			}
		}
		cands = append(cands, cand{idx: i, tier: tier, rem: counts[key], posOrder: i})
	}
	return &candidateOrder{cands: cands}
}

// next returns the next-best candidate index, or ok=false when exhausted.
//
// The FIRST call selects the minimum in one O(n) pass and swaps it to the front — no sort.
// A SECOND call means the first choice dead-ended, so the remainder is sorted once and served
// from there. The result is identical to sorting up front, because both use candLess and the
// comparator is a total order (posOrder is unique).
func (c *candidateOrder) next() (int, bool) {
	if c.i >= len(c.cands) {
		return 0, false
	}
	switch {
	case c.i == 0:
		best := 0
		for j := 1; j < len(c.cands); j++ {
			if candLess(c.cands[j], c.cands[best]) {
				best = j
			}
		}
		c.cands[0], c.cands[best] = c.cands[best], c.cands[0]
	case !c.sorted:
		// The first pick failed. Sort what remains ONCE; every later call is a cursor read.
		rest := c.cands[c.i:]
		sort.Slice(rest, func(a, b int) bool { return candLess(rest[a], rest[b]) })
		c.sorted = true
	}
	idx := c.cands[c.i].idx
	c.i++
	return idx, true
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
		for _, slot := range slices.Backward(out) {
			if seriesKeyOf(slot) == key {
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
		for _, slot := range slices.Backward(out) {
			if seriesKeyOf(slot) == key {
				sinceMs = accMs
				break
			}
			accMs += slot.DurationMs
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
