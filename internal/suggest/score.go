package suggest

import (
	"strings"
)

// This file holds the DETERMINISTIC post-scoring layered on the LLM output (§8):
// given the same intent + same picks, score() always returns the same Scores, so
// ranking is reproducible and testable — "not pure vibes". Criteria are simple
// and explainable (SmarTunarr-style multi-criterion), and the weights are
// configurable in spirit (v1 hardcodes a sensible default; §8 says "keep criteria
// configurable" — a follow-on knob, not this phase).

// score computes the deterministic scores for a proposal's lineup + acquisitions.
// All sub-scores are in [0,1].
func score(intent Intent, lineup, acquisitions []ProposalItem) Scores {
	total := len(lineup) + len(acquisitions)
	if total == 0 {
		return Scores{}
	}
	s := Scores{
		ThemeFit:          themeFit(intent, lineup, acquisitions),
		AvailabilityRatio: float64(len(lineup)) / float64(total),
		EraBalance:        eraBalance(intent, lineup, acquisitions),
	}
	s.Overall = composite(s)
	return s
}

// themeFit measures how many items' names contain any intent term (description +
// must-include terms). A blunt but explainable proxy for "matches the ask".
func themeFit(intent Intent, lineup, acquisitions []ProposalItem) float64 {
	terms := themeTerms(intent)
	if len(terms) == 0 {
		return 1 // no terms to fit against → neutral-max
	}
	items := append(append([]ProposalItem{}, lineup...), acquisitions...)
	if len(items) == 0 {
		return 0
	}
	hits := 0
	for _, it := range items {
		name := strings.ToLower(it.Name)
		for _, term := range terms {
			if strings.Contains(name, term) {
				hits++
				break
			}
		}
	}
	return float64(hits) / float64(len(items))
}

// themeTerms extracts lowercase significant words from the intent (description +
// must-include), skipping trivial stopwords so "a channel of" doesn't count.
func themeTerms(intent Intent) []string {
	var raw []string
	raw = append(raw, strings.Fields(strings.ToLower(intent.Description))...)
	for _, m := range intent.MustInclude {
		raw = append(raw, strings.ToLower(m))
	}
	stop := map[string]bool{"a": true, "an": true, "the": true, "of": true, "and": true, "for": true, "channel": true, "movies": true, "shows": true}
	var terms []string
	for _, w := range raw {
		w = strings.Trim(w, ".,!?\"'")
		if len(w) < 3 || stop[w] {
			continue
		}
		terms = append(terms, w)
	}
	return terms
}

// eraBalance rewards a spread of years when the intent targets an era. With no
// era or one item it's neutral (1). The metric: fraction of distinct decades
// among items relative to items — higher = better spread. When the intent names
// an era, items outside it are penalized by not counting toward the spread.
func eraBalance(intent Intent, lineup, acquisitions []ProposalItem) float64 {
	items := append(append([]ProposalItem{}, lineup...), acquisitions...)
	if len(items) <= 1 {
		return 1
	}
	decades := map[int]bool{}
	withYear := 0
	for _, it := range items {
		if it.Year <= 0 {
			continue
		}
		withYear++
		decades[it.Year/10] = true
	}
	if withYear == 0 {
		return 1 // no year info → don't penalize
	}
	// Spread = distinct decades / items-with-year, clamped to [0,1].
	spread := float64(len(decades)) / float64(withYear)
	if spread > 1 {
		spread = 1
	}
	return spread
}

// composite weights the sub-scores into the overall ranking score.
//
// TODO(learning): implement composite(). This weighting decides how proposals
// rank against each other — the single most product-shaping number in the
// suggester. Return a weighted average of s.ThemeFit, s.AvailabilityRatio, and
// s.EraBalance in [0,1].
//
// The trade-off to weigh:
//   - ThemeFit is "does it match what they asked for" — arguably the point.
//   - AvailabilityRatio is "how much is playable RIGHT NOW" — a channel that's
//     90% acquisitions is dead air for days (§9 never-dead-air is downstream, but
//     ranking should prefer proposals that light up fast).
//   - EraBalance is a nice-to-have polish signal.
//
// Theme-first weighting (maintainer decision): matching the ask dominates, with
// how-much-is-playable-now a strong second and era spread a light tiebreaker.
// Weights sum to 1.0 so Overall stays in [0,1].
func composite(s Scores) float64 {
	return 0.5*s.ThemeFit + 0.35*s.AvailabilityRatio + 0.15*s.EraBalance
}
