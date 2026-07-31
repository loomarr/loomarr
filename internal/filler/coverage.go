package filler

// Coverage answers, for a given block target, WHICH RUNG OF THE LADDER a break would be
// filled from and how much material each rung holds (§10, plan §6 + §6.2).
//
// ⚠ **The whole point is that this does not reimplement the ladder.** The v2 mock's coverage
// meter claims to come "from the same ladder reconcile uses" and does not: it recomputes its
// buckets inline, with five mutually inconsistent era/audience predicates. A meter that
// disagrees with what actually airs is worse than no meter — it is a confident wrong answer
// about why breaks look the way they do. So `Coverage` calls `candidatePools`, the same
// unexported builder `Assemble` calls, and reports what it finds. If the ladder changes, this
// changes with it or fails to compile; there is no second copy to drift.
//
// It reports pools rather than drawing from them, so it is independent of Seed, no-repeat and
// category variety — those decide WHICH clip plays, not whether the rung can fill a break.

// RungCoverage is one ladder rung and how much material it holds for a target.
type RungCoverage struct {
	// Level is the rung — exact / widened / audience. Same vocabulary as Pod.MatchLevel,
	// so a coverage answer and a pod outcome are directly comparable.
	Level MatchLevel
	// Clips is how many DURATION-ELIGIBLE clips this rung holds. Ineligible clips are
	// already gone: a 90-second spot cannot fill a 30-second break, and counting it would
	// promise coverage that assembly cannot deliver.
	Clips int
}

// CoverageReport is what a target's breaks would draw from.
type CoverageReport struct {
	// Rungs, tightest first, in ladder order. A rung the policy skips is ABSENT rather
	// than present-and-zero — under EraStrict there is no "widened" rung to be empty,
	// and rendering one at 0 would read as a catalog gap instead of a setting.
	Rungs []RungCoverage
	// Level is the rung a break would actually be filled from — the tightest non-empty
	// one — matching what Assemble would report as Pod.MatchLevel for the same input.
	// MatchBumperCard when no rung has anything, which is the honest "this break would be
	// the embedded card" answer rather than a zero.
	Level MatchLevel
	// Total is the eligible commercial count across the WIDEST rung, i.e. everything that
	// could fill this break at any match quality. Not a sum: the rungs nest (exact ⊆
	// widened ⊆ audience), so adding them counts the same clip up to three times.
	Total int
}

// Coverage reports which rung a break for `w` would draw from, given `catalog`.
//
// ⚠ The catalog is narrowed in the SAME ORDER Assemble narrows it — excluded ids, then kinds,
// then the pools — because that order is load-bearing. Kinds is applied catalog-wide (it shapes
// bumpers too), and exclusions must be gone before pools are counted or coverage reports clips
// that assembly will never pick.
func Coverage(catalog []Clip, w Window, policy Policy) CoverageReport {
	excluded := make(map[string]bool, len(w.Excluded))
	for _, id := range w.Excluded {
		excluded[id] = true
	}
	if len(excluded) > 0 {
		kept := make([]Clip, 0, len(catalog))
		for _, c := range catalog {
			if !excluded[c.ID()] {
				kept = append(kept, c)
			}
		}
		catalog = kept
	}
	catalog = filterKinds(catalog, w.Kinds)

	pools := candidatePools(catalog, w, policy)

	report := CoverageReport{
		Rungs: make([]RungCoverage, 0, len(pools)),
		Level: MatchBumperCard,
	}
	for _, p := range pools {
		report.Rungs = append(report.Rungs, RungCoverage{Level: p.level, Clips: len(p.clips)})
		// Tightest non-empty wins, and only the first one — the ladder never widens
		// further than it must, so a later rung being larger does not change the answer.
		if len(p.clips) > 0 && report.Level == MatchBumperCard {
			report.Level = p.level
		}
	}
	if n := len(report.Rungs); n > 0 {
		// The widest rung is the last, and the rungs nest, so its count is the total
		// distinct material — summing would triple-count an exact-era clip.
		report.Total = report.Rungs[n-1].Clips
	}
	return report
}
