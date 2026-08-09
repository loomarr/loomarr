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

// Criterion names one of the channel's break settings, for the per-criterion breakdown.
//
// ⚠ **A separate vocabulary from `FitReason`, deliberately.** `FitReason` answers "why was THIS
// clip rejected", and `era` is not in it because an out-of-era clip is never rejected — it drops
// a rung and still plays. A criterion is a SETTING the operator can change, which is a different
// question with a different membership.
type Criterion string

const (
	CriterionEra      Criterion = "era"
	CriterionAudience Criterion = "audience"
	CriterionCategory Criterion = "category"
	CriterionKind     Criterion = "kind"
	CriterionDuration Criterion = "duration"
	CriterionQuality  Criterion = "quality"
)

// CriterionCoverage is one setting and how much of the commercial catalog survives it ON ITS OWN.
//
// ⚠ **Independent, not cumulative, and that is the entire value.** The question an operator has
// is "which of my settings is costing me the clips" — and a cumulative funnel cannot answer it,
// because the order the predicates happen to run in decides which setting gets the blame. Counting
// each in isolation over the same base means "Audience: 0" identifies the culprit rather than
// reporting a race.
//
// ⚠ **It is also why this cannot be derived from `FitFor`, which the V51f plan proposed.** FitFor
// short-circuits on the first failing predicate, so a clip failing both category and audience is
// attributed to whichever check is written first — an answer that would change if someone
// reordered that function, with nothing to catch it.
type CriterionCoverage struct {
	Criterion Criterion
	// Clips is how many break-body commercials pass THIS criterion, ignoring the others.
	Clips int
}

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
	// Criteria is the per-setting breakdown — which single setting is emptying the ladder.
	// Always the same six entries in the same order, so the UI renders a stable list rather
	// than one that reshuffles as counts change.
	Criteria []CriterionCoverage
	// Rungs, tightest first, in ladder order.
	//
	// ⚠ Since V51f every rung is always present: `EraStrict` (retired-ok) was the only thing that could skip
	// one, and it was unreachable — set in tests and nowhere else. The absent-not-zero rule it
	// justified is gone with it, so a rung at 0 now means what a reader would assume it means:
	// nothing in the catalog reaches that rung.
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

// ChannelCoverage is one channel's coverage answer, labelled for display (§10 V35).
//
// It carries the channel's identity as plain strings/ints rather than a channel type: this
// package must not learn about channels, and a name is all the pool strip needs to say WHICH
// channel is thin.
type ChannelCoverage struct {
	ChannelID string
	Name      string
	Number    int
	Report    CoverageReport
}

// PoolReport is catalog-wide filler health (§10 V35) — the Filler page's pool strip.
//
// ⚠ **The per-channel half is not a new computation.** `Channels` is filled by calling the SAME
// `CoverageFor` the per-channel meter calls, once per live channel, so the strip and the channel
// page cannot disagree — there is no aggregate ladder to drift from the real one. The
// catalog-wide counts below are the only thing computed here, and they are counts, not matching.
type PoolReport struct {
	// Clips is every row in the catalog, of every kind.
	Clips int
	// Commercials is the subset that fills a break BODY. Bumpers and station IDs bookend a
	// pod; a catalog of nothing but bumpers cannot make one.
	Commercials int
	// Eligible is the commercials that are also DURATION-eligible under the active policy.
	//
	// ⚠ This is the number that surprises operators, which is why it is a headline rather than
	// a footnote: a catalog of 500 fifteen-minute compilations reads as healthy by `Clips` and
	// can fill nothing. It is the same `durationEligible` gate assembly applies.
	Eligible int
	// Untagged is commercials missing a match tag, counted by the STORE.
	//
	// ⚠ Counted there on purpose. "Untagged" is defined once, as SQL, in `store/clips.go` (the
	// AI-tagging work list reads the same predicate); recomputing it in Go here would be a
	// second definition of the word, free to drift from the job that acts on it.
	Untagged int
	// Channels is per-live-channel coverage, worst first, so the strip's diagnosis line can
	// name a channel without the caller sorting. Empty on an install with no live channels —
	// which is a real state (a fresh install), not a missing answer.
	Channels []ChannelCoverage
}

// Weakest returns the channel in the worst shape, or nil when there are none.
//
// "Worst" is ladder order — a channel falling through to the bumper card is worse than one
// widening its era — because that is the order in which an operator would fix them.
func (p PoolReport) Weakest() *ChannelCoverage {
	if len(p.Channels) == 0 {
		return nil
	}
	return &p.Channels[0]
}

// LevelWorseThan reports whether `a` is a worse coverage outcome than `b`.
//
// ⚠ **This is an ordering for presentation, not a scale.** MatchLevel says which rung the ladder
// stopped at; it is not a score, and nothing in assembly compares two levels. It lives here
// rather than in a UI so that "worse" means the ladder's own order — a channel falling through
// to the bumper card is worse than one that merely widened its era, because that is the order an
// operator would fix them in.
func LevelWorseThan(a, b MatchLevel) bool { return levelRank(a) < levelRank(b) }

// levelRank orders match levels worst-first. Unexported: callers should ask the question
// (LevelWorseThan) rather than read a number that would invite arithmetic on it.
func levelRank(l MatchLevel) int {
	switch l {
	case MatchBumperCard:
		return 0
	case MatchAudience:
		return 1
	case MatchWidened:
		return 2
	case MatchExact:
		return 3
	default:
		// An unknown level sorts BEST, so a future rung nobody taught this function about
		// cannot silently become "the channel to fix" at the top of the operator's list.
		return 4
	}
}

// PoolCounts computes the catalog-wide half of a PoolReport (§10 V35).
//
// Pure and channel-free, so it is testable without a store and cannot accidentally depend on
// which channels happen to exist. `Untagged` is deliberately NOT set here — see the field.
func PoolCounts(catalog []Clip, policy Policy) PoolReport {
	report := PoolReport{Clips: len(catalog)}
	for _, c := range catalog {
		if c.Kind != Commercial {
			continue
		}
		report.Commercials++
		if durationEligible(c, policy) {
			report.Eligible++
		}
	}
	return report
}

// criterionCoverage counts, per setting, how much break-body material survives that setting alone.
//
// ⚠ **Every count comes from the predicate the LADDER uses, never a re-derived one.** This is the
// same rule `Coverage` itself follows and for the same reason: `coverage.go` exists because the
// mock's meter recomputed its buckets inline with five mutually inconsistent era/audience tests. A
// per-criterion breakdown that disagrees with what airs is a more specific version of exactly that
// confident wrong answer — it names a setting to change.
//
// The base is break-BODY material: commercials only, after excluded ids are gone. Bumpers bookend
// a pod and are not what an operator means by "my clips".
func criterionCoverage(catalog []Clip, w Window, policy Policy) []CriterionCoverage {
	var body []Clip
	for _, c := range catalog {
		if c.Kind == Commercial {
			body = append(body, c)
		}
	}

	count := func(keep func(Clip) bool) int {
		n := 0
		for _, c := range body {
			if keep(c) {
				n++
			}
		}
		return n
	}

	return []CriterionCoverage{
		{CriterionEra, count(func(c Clip) bool { return w.Era.Contains(c.Era) })},
		{CriterionAudience, count(func(c Clip) bool {
			return len(filterAudienceWithUngrounded([]Clip{c}, w.Audience)) > 0
		})},
		{CriterionCategory, count(func(c Clip) bool {
			return len(filterCategories([]Clip{c}, w.Categories)) > 0
		})},
		{CriterionKind, count(func(c Clip) bool {
			return len(filterKinds([]Clip{c}, w.Kinds)) > 0
		})},
		{CriterionDuration, count(func(c Clip) bool { return durationEligible(c, policy) })},
		{CriterionQuality, count(func(c Clip) bool { return qualityEligible(c, policy) })},
	}
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
	// ⚠ The per-criterion breakdown is measured BEFORE the kind filter, because "kind" is itself
	// one of the criteria being reported. Measuring it after would make that line always equal the
	// base — a row that can never say anything.
	criteria := criterionCoverage(catalog, w, policy)

	catalog = filterKinds(catalog, w.Kinds)

	pools := candidatePools(catalog, w, policy)

	report := CoverageReport{
		Criteria: criteria,
		Rungs:    make([]RungCoverage, 0, len(pools)),
		Level:    MatchBumperCard,
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
