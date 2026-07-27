package schedule

import (
	"fmt"
	"time"
)

// The relaxation ladder (programming-design §7): when the eligible pool can't fill
// the cycle window under the policy's separation constraints, enforcement descends
// a FIXED ladder, re-attempting placement after each step, and RECORDS every
// applied relaxation so the UI can surface it. Audience and explicit scope are
// filtered out BEFORE the pool reaches here (filterEntries), so the ladder
// structurally can never relax safety or channel identity (§7 "never relaxed").
//
// Un-relaxing is automatic: this recomputes from the full policy each call, so when
// the pool recovers (library grows) a later reconcile simply produces fewer/zero
// applied steps.

// minNoRepeatFloor is the hard floor for the no-repeat window relaxation (§7 step 1).
const minNoRepeatFloor = 24 * time.Hour

// eraWidenStep is how far the era widens per ladder step (§7 step 3).
const eraWidenStep = 2

// maxLadderSteps bounds ladder descent (defense-in-depth; each real step also has
// its own floor/limit).
const maxLadderSteps = 8

// relaxAndSlot places the slots under the policy, descending the relaxation ladder
// only if the placement can't satisfy separation across the cycle (§7). It returns
// the ordered slots and the list of relaxations it had to apply (empty when the
// pool satisfied the policy as-is). Deterministic: the seed drives all ordering,
// and the ladder descends in a fixed order.
func relaxAndSlot(slots []Slot, rp ResolvedPolicy, seed int64) ([]Slot, []AppliedRelaxation) {
	// First attempt: place under the policy as written.
	ordered := slotByPolicy(slots, rp, seed)
	if !separationUnsatisfied(ordered, rp) {
		return ordered, nil
	}

	var applied []AppliedRelaxation
	cur := rp
	for step := 0; step < maxLadderSteps; step++ {
		relaxed, note, ok := ladderStep(cur)
		if !ok {
			break // ladder exhausted — hand off to filler (the caller pads; never dead air)
		}
		prev := cur
		cur = relaxed
		applied = append(applied, note)

		// Re-place ONLY when the step changed something placement actually reads.
		//
		// The ladder's first steps halve the no-repeat WINDOWS (168h → 84h → 42h → 24h), and
		// `noRepeatUnmet` is pure arithmetic over the cycle's total runtime — it compares a
		// duration, it does not arrange anything. Placement reads BlockMax, SeriesMinGap,
		// scope and ordering; it never reads the no-repeat windows. So re-running
		// slotByPolicy after a no-repeat-only step re-derives a byte-identical arrangement.
		//
		// Measured on a 100-slot series channel: steps 1-3 each re-ran the full backtracking
		// search (~6-10ms apiece) and produced placementChanged=false every time. Only the
		// step that relaxed SeriesMinGap changed the answer — and it was the cheapest, at
		// 817µs, because dropping the constraint makes the search trivial.
		//
		// This is NOT a semantics change: when the inputs are equal the output is equal by
		// construction, so the arrangement, the applied-relaxation list and the final verdict
		// are all exactly what the old code produced. It only stops paying for the same
		// answer repeatedly.
		if placementInputsChanged(prev, cur) {
			ordered = slotByPolicy(slots, cur, seed)
		}
		if !separationUnsatisfied(ordered, cur) {
			break
		}
	}
	return ordered, applied
}

// placementInputsChanged reports whether a ladder step altered anything slotByPolicy consumes.
//
// Deliberately a WHITELIST of the fields placement reads rather than a `!=` on the whole
// policy: a future ladder step that relaxes some other field must be considered here on
// purpose. Getting it wrong in the safe direction (reporting a change that did not happen)
// costs one redundant placement; getting it wrong the other way would serve a stale
// arrangement, so the comparison is explicit and narrow.
//
// Not included, deliberately: EpisodeNoRepeat / MovieNoRepeat. They gate `noRepeatUnmet`, which
// is a runtime comparison over the already-placed cycle — see the call site.
func placementInputsChanged(prev, cur ResolvedPolicy) bool {
	if prev.Sep.BlockMax != cur.Sep.BlockMax || prev.Sep.SeriesMinGap != cur.Sep.SeriesMinGap {
		return true
	}
	if prev.Ordering != cur.Ordering {
		return true
	}
	// Era is the scope field the ladder widens; compare by value (both may be nil).
	switch {
	case prev.Scope.Era == nil && cur.Scope.Era == nil:
		return false
	case prev.Scope.Era == nil || cur.Scope.Era == nil:
		return true
	default:
		return *prev.Scope.Era != *cur.Scope.Era
	}
}

// separationUnsatisfied reports whether the placed cycle can't honor the policy
// after slotting — the signal to descend the ladder (§7). Two independent shortfalls:
//
//   - texture separation (blockMax/seriesMinGap) breached across the cycle, per the
//     wrap-aware checker; and
//   - the no-repeat WINDOW can't be met: the cycle's total program runtime is
//     shorter than episodeNoRepeat/movieNoRepeat, so an item necessarily recurs
//     inside its window each loop. This is a runtime comparison, not a graph check —
//     the case a single-series channel (block unbounded) hits.
func separationUnsatisfied(programs []Slot, rp ResolvedPolicy) bool {
	if len(checkWrapSeparation(programs, rp)) > 0 {
		return true
	}
	return noRepeatUnmet(programs, rp)
}

// noRepeatUnmet reports whether the cycle loops faster than the no-repeat window
// requires (§3/§7 step 1). The window an item must clear is the larger of the
// episode/movie no-repeat durations that apply to the pool; if the cycle's total
// runtime is shorter, the item repeats within its window on every loop.
func noRepeatUnmet(programs []Slot, rp ResolvedPolicy) bool {
	var cycleMs int64
	hasSeries, hasMovie := false, false
	for _, s := range programs {
		if !s.IsProgram() {
			continue
		}
		cycleMs += s.DurationMs
		if s.Key.IsSeries() {
			hasSeries = true
		} else {
			hasMovie = true
		}
	}
	if cycleMs == 0 {
		return false // nothing to schedule → not a relaxation case (handled as flex)
	}
	var window time.Duration
	if hasSeries && rp.Sep.EpisodeNoRepeat > window {
		window = rp.Sep.EpisodeNoRepeat
	}
	if hasMovie && rp.Sep.MovieNoRepeat > window {
		window = rp.Sep.MovieNoRepeat
	}
	return window > 0 && time.Duration(cycleMs)*time.Millisecond < window
}

// ladderStep applies the next relaxation in the fixed ladder order (§7) to rp,
// returning the relaxed policy, a display note, and ok=false when the ladder can't
// descend further. Order:
//  1. halve episode/movie no-repeat windows (floor 24h)
//  2. relax series gap / block (toward unbounded)
//  3. widen era ±2y per step (never past a stated decade boundary)
//  4. (caller) pad with filler — represented by ok=false here
//
// NEVER touched: audience ceiling, explicit series/season scope (they aren't even
// fields this function reads).
func ladderStep(rp ResolvedPolicy) (ResolvedPolicy, AppliedRelaxation, bool) {
	// Step 1: no-repeat windows.
	if rp.Sep.EpisodeNoRepeat > minNoRepeatFloor {
		from := rp.Sep.EpisodeNoRepeat
		rp.Sep.EpisodeNoRepeat = halveFloor(rp.Sep.EpisodeNoRepeat)
		rp.Sep.MovieNoRepeat = halveFloor(rp.Sep.MovieNoRepeat)
		return rp, AppliedRelaxation{Kind: "episodeNoRepeat", From: from.String(), To: rp.Sep.EpisodeNoRepeat.String()}, true
	}
	// Step 2: series gap / block.
	if rp.Sep.SeriesMinGap > 0 {
		from := rp.Sep.SeriesMinGap
		rp.Sep.SeriesMinGap = 0
		return rp, AppliedRelaxation{Kind: "seriesMinGap", From: from.String(), To: "0s"}, true
	}
	if rp.Sep.BlockMax > 0 {
		from := rp.Sep.BlockMax
		rp.Sep.BlockMax = 0 // unbounded
		return rp, AppliedRelaxation{Kind: "blockMax", From: fmt.Sprintf("%d", from), To: "unbounded"}, true
	}
	// Step 3: widen era (only when an era is set). Widening is symmetric ±2y; a
	// stated decade boundary is respected by not widening past the decade the era
	// was authored in — v1 keeps this simple and widens once per call.
	if rp.Scope.Era != nil && (rp.Scope.Era.From > 0 || rp.Scope.Era.To > 0) {
		from := *rp.Scope.Era
		widened := widenEra(from)
		if widened != from {
			rp.Scope.Era = &widened
			return rp, AppliedRelaxation{
				Kind: "era",
				From: fmt.Sprintf("%d-%d", from.From, from.To),
				To:   fmt.Sprintf("%d-%d", widened.From, widened.To),
			}, true
		}
	}
	// Ladder exhausted — the caller pads with filler (never dead air), no more
	// separation relaxation possible.
	return rp, AppliedRelaxation{}, false
}

// halveFloor halves d but never below the no-repeat floor (§7 step 1).
func halveFloor(d time.Duration) time.Duration {
	h := d / 2
	if h < minNoRepeatFloor {
		return minNoRepeatFloor
	}
	return h
}

// widenEra widens an era window by ±eraWidenStep years, clamping to the decade the
// window sits in so we never cross a stated decade boundary (§7 step 3). Returns the
// input unchanged when already at the decade edges.
func widenEra(r Range) Range {
	out := r
	if r.From > 0 {
		decadeStart := (r.From / 10) * 10
		out.From = r.From - eraWidenStep
		if out.From < decadeStart {
			out.From = decadeStart
		}
	}
	if r.To > 0 {
		decadeEnd := (r.To/10)*10 + 9
		out.To = r.To + eraWidenStep
		if out.To > decadeEnd {
			out.To = decadeEnd
		}
	}
	return out
}
