package schedule

import (
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
)

// Relaxation (programming-design §7): when the eligible pool can't hold the policy, each guard the
// placed cycle breaches relaxes on its own, to the widest value the pool can hold, and every
// relaxation is recorded so the UI can surface it. A shortfall in one guard never touches another
// (#4140 decision 3): a short cycle keeps the series gap, and a gap breach keeps the no-repeat
// windows.
//
// Audience and scope are filtered out BEFORE the pool reaches here (filterEntries), so nothing
// here can relax safety or the channel's line; the dates never widen either. A channel short of
// titles says so, and growth is what widens a pool.
//
// Un-relaxing is automatic: this recomputes from the full policy each call, so when the pool
// recovers a later reconcile records fewer relaxations, or none.

// relaxAndSlot places the slots under the policy and relaxes each guard the placement breaches
// (§7). It returns the ordered slots and the relaxations it applied, empty when the pool held the
// policy as written. Deterministic: the seed drives all ordering.
func relaxAndSlot(slots []Slot, rp ResolvedPolicy, seed int64) ([]Slot, []AppliedRelaxation) {
	ordered := slotByPolicy(slots, rp, seed)
	applied := relaxNoRepeat(ordered, rp.Sep)

	// The gap is settled before the run length: a run of two always breaches a gap, so raising
	// the run cap first could never help a gap, and a gap settled first shows which runs remain.
	if breaches(ordered, rp, "gap") {
		gap := widestGap(slots, rp, seed)
		applied = append(applied, AppliedRelaxation{Kind: "seriesMinGap", From: rp.Sep.SeriesMinGap.String(), To: gap.String()})
		rp.Sep.SeriesMinGap = gap
		ordered = slotByPolicy(slots, rp, seed)
	}
	if breaches(ordered, rp, "block") {
		run := shortestRun(slots, rp, seed)
		to := "unbounded"
		if run > 0 {
			to = fmt.Sprintf("%d", run)
		}
		applied = append(applied, AppliedRelaxation{Kind: "blockMax", From: fmt.Sprintf("%d", rp.Sep.BlockMax), To: to})
		rp.Sep.BlockMax = run
		ordered = slotByPolicy(slots, rp, seed)
	}
	return ordered, applied
}

// relaxNoRepeat records the no-repeat windows the cycle can't keep. An item airs once per loop,
// so a window longer than the cycle's program runtime relaxes to that runtime, to the minute.
// Placement never reads these windows, so this is arithmetic over the placed cycle. Episodes and
// films are judged apart: each window applies only when the pool holds that kind.
func relaxNoRepeat(programs []Slot, sep ResolvedSeparation) []AppliedRelaxation {
	var cycleMs int64
	episodes, films := false, false
	for _, s := range programs {
		if !s.IsProgram() {
			continue
		}
		cycleMs += s.DurationMs
		if s.Key.IsSeries() {
			episodes = true
		} else {
			films = true
		}
	}
	cycle := time.Duration(cycleMs) * time.Millisecond
	if cycle <= 0 {
		return nil // nothing to schedule → not a relaxation case (handled as flex)
	}
	held := cycle.Truncate(time.Minute)
	if held == 0 {
		held = cycle
	}
	var out []AppliedRelaxation
	if episodes && sep.EpisodeNoRepeat > cycle {
		out = append(out, AppliedRelaxation{Kind: "episodeNoRepeat", From: sep.EpisodeNoRepeat.String(), To: held.String()})
	}
	if films && sep.MovieNoRepeat > cycle {
		out = append(out, AppliedRelaxation{Kind: "movieNoRepeat", From: sep.MovieNoRepeat.String(), To: held.String()})
	}
	return out
}

// breaches reports whether the placed cycle breaks the separation guard of the given violation
// kind ("gap" or "block"), across the loop seam.
func breaches(programs []Slot, rp ResolvedPolicy, kind string) bool {
	for _, v := range checkWrapSeparation(programs, rp) {
		if v.Kind == kind {
			return true
		}
	}
	return false
}

// widestGap is the widest series gap, to the minute and below the policy's, that a placement of
// the slots holds; zero when nothing wider does.
//
// No placement can do better than the tightest show's share: a show with k slots spends the rest
// of the cycle in k gaps, so its narrowest gap is at most (cycle - its runtime) / k. That bound is
// tried first, and is the answer whenever the placement spreads the show evenly.
//
// Below it the search scans down in gapStep steps, then back up a minute at a time. A binary
// search would assume that a gap which fails means every wider gap fails too, and the placer is a
// heuristic that does not keep that promise: it can miss 52 minutes and find an hour. A probe
// that fails costs only its capped search, so the scan stays cheap.
func widestGap(slots []Slot, rp ResolvedPolicy, seed int64) time.Duration {
	holds := func(minutes int64) bool {
		try := probing(rp, slots)
		try.Sep.SeriesMinGap = time.Duration(minutes) * time.Minute
		return !breaches(slotByPolicy(slots, try, seed), try, "gap")
	}
	top := min(rp.Sep.SeriesMinGap-time.Minute, gapBound(slots)).Milliseconds() / time.Minute.Milliseconds()
	if top <= 0 {
		return 0
	}
	if holds(top) {
		return time.Duration(top) * time.Minute
	}
	found := int64(0)
	for m := (top - 1) / gapStep * gapStep; m > 0; m -= gapStep {
		if holds(m) {
			found = m
			break
		}
	}
	if found == 0 {
		return 0
	}
	for m := found + 1; m < min(found+gapStep, top) && holds(m); m++ {
		found = m
	}
	return time.Duration(found) * time.Minute
}

// gapStep is widestGap's coarse step, in minutes.
const gapStep = 5

// gapBound is the widest series gap the slots could hold in any arrangement (see widestGap).
func gapBound(slots []Slot) time.Duration {
	counts, runtime, cycle := showShares(slots)
	bound := time.Duration(cycle) * time.Millisecond
	for key, k := range counts {
		bound = min(bound, time.Duration((cycle-runtime[key])/int64(k))*time.Millisecond)
	}
	return bound
}

// shortestRun is the shortest back-to-back run above the policy's that a placement of the slots
// holds; zero (unbounded) when only the largest show's whole run would.
//
// No placement can do better than spreading the largest share evenly: a show with k slots among m
// others runs at least ceil(k/m) in a row, and one show alone runs its whole length. That bound
// is tried first; past it the run doubles until one holds, and a binary search settles between
// the last that failed and the first that held, so a long show among few others costs a handful
// of probes rather than one per episode.
func shortestRun(slots []Slot, rp ResolvedPolicy, seed int64) int {
	counts, _, _ := showShares(slots)
	if len(counts) <= 1 {
		return 0
	}
	total, largest, floor := 0, 0, rp.Sep.BlockMax+1
	for _, k := range counts {
		total += k
		largest = max(largest, k)
	}
	for _, k := range counts {
		floor = max(floor, (k+total-k-1)/(total-k))
	}
	holds := func(run int) bool {
		if run >= largest {
			return true // a cap no show can reach is no cap
		}
		try := probing(rp, slots)
		try.Sep.BlockMax = run
		return !breaches(slotByPolicy(slots, try, seed), try, "block")
	}
	if holds(floor) {
		return unboundedAt(floor, largest)
	}
	failed, step := floor, 1
	for !holds(failed + step) {
		failed += step
		step *= 2
	}
	held := failed + step
	for held-failed > 1 {
		mid := failed + (held-failed)/2
		if holds(mid) {
			held = mid
		} else {
			failed = mid
		}
	}
	return unboundedAt(held, largest)
}

// unboundedAt is run as a run cap, zero (unbounded) when no show is long enough to reach it.
func unboundedAt(run, largest int) int {
	if run >= largest {
		return 0
	}
	return run
}

// probing is rp with the search capped for a probe (see ResolvedPolicy.searchBudget). A probe
// that holds places exactly as the uncapped search would, since a capped search only stops
// sooner; one that does not hold may have been solvable past the cap, so the guard settles a
// little tighter than the widest the full search could reach.
func probing(rp ResolvedPolicy, slots []Slot) ResolvedPolicy {
	rp.searchBudget = 32*len(slots) + 256
	return rp
}

// showShares counts each show's program slots and runtime, and the cycle's program runtime (ms).
func showShares(slots []Slot) (map[provision.Key]int, map[provision.Key]int64, int64) {
	counts := map[provision.Key]int{}
	runtime := map[provision.Key]int64{}
	var cycle int64
	for _, s := range slots {
		if !s.IsProgram() {
			continue
		}
		key := seriesKeyOf(s)
		counts[key]++
		runtime[key] += s.DurationMs
		cycle += s.DurationMs
	}
	return counts, runtime, cycle
}
