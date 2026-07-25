package schedule

import (
	"reflect"
	"sort"
)

// Policy ownership merge (programming-design §2.1 / §8.2). ChannelPolicy mixes three
// owners — proposal-extracted, operator-edited, and reconcile-output — and this file is
// the ONE place that ownership is enforced. Every writer (the binder on approve/refine/
// re-curation, and the API on an operator PATCH) goes through these methods, so no writer
// hand-restores fields, and a new field is preserved by default (out := current) unless a
// method deliberately overwrites it.

// The OperatorSet field-paths — the proposal-owned fields an operator can pin by editing.
// Field-level granularity (whole scope/audience/…): editing any sub-field pins the field.
const (
	pathScope      = "scope"
	pathAudience   = "audience"
	pathSeparation = "separation"
	pathOrdering   = "ordering"
	pathSeasonal   = "seasonal"
)

// MergeFromProposal materializes a fresh proposal onto the channel (approve / refine /
// re-curation). Proposal-owned fields refresh from `incoming` UNLESS the operator has
// pinned them (OperatorSet, §8.2 stickiness); operator-owned fields (filler/window/
// autoCurate/playout), Applied, and OperatorSet are preserved from current via
// `out := current`. Playout backend (§9.1) is an OPERATOR choice about infrastructure,
// never a creative one about content — a refine must not be able to move a channel
// between streaming backends, so it is deliberately absent from the refresh list below.
// Rules merge by provenance. The audience ceiling is NEVER relaxed — even a pinned ceiling
// may be tightened by a stricter proposal, never loosened (§4 safety outranks stickiness).
func (current ChannelPolicy) MergeFromProposal(incoming ChannelPolicy) ChannelPolicy {
	out := current // keeps OperatorPolicy, Applied, OperatorSet, and any future field
	pinned := pathSet(current.OperatorSet)

	if !pinned[pathScope] {
		out.Scope = incoming.Scope
	}
	if pinned[pathAudience] {
		// Pinned: operator's audience is sticky — but a STRICTER proposal ceiling still
		// applies (you can always get safer), never a looser one (§4).
		out.Audience = current.Audience
		out.Audience.Ceiling = tighterCeiling(current.Audience.Ceiling, incoming.Audience.Ceiling)
	} else {
		out.Audience = incoming.Audience
	}
	if !pinned[pathSeparation] {
		out.Separation = incoming.Separation
	}
	if !pinned[pathOrdering] {
		out.Ordering = incoming.Ordering
	}
	if !pinned[pathSeasonal] {
		out.Seasonal = incoming.Seasonal
	}
	out.Rules = mergeRules(current.Rules, incoming.Rules)
	return out
}

// MergeFromOperator applies an operator PATCH of the whole policy (the channel page). The
// operator's values win wholesale; Applied is reconcile-owned so it is force-preserved from
// current (never client-settable); and every proposal-owned field the operator CHANGED is
// recorded in OperatorSet so a later refine can't revert it (§8.2). OperatorSet on the
// incoming body is ignored — it is Loomarr's bookkeeping, not a client-settable field.
func (current ChannelPolicy) MergeFromOperator(incoming ChannelPolicy) ChannelPolicy {
	out := incoming
	out.Applied = current.Applied // reconcile-owned

	set := pathSet(current.OperatorSet)
	if !reflect.DeepEqual(current.Scope, incoming.Scope) {
		set[pathScope] = true
	}
	if current.Audience != incoming.Audience {
		set[pathAudience] = true
	}
	if current.Separation != incoming.Separation {
		set[pathSeparation] = true
	}
	if current.Ordering != incoming.Ordering {
		set[pathOrdering] = true
	}
	if !reflect.DeepEqual(current.Seasonal, incoming.Seasonal) {
		set[pathSeasonal] = true
	}
	out.OperatorSet = sortedPaths(set)
	return out
}

// WithApplied is the reconcile writer: it may write ONLY the relaxation record, leaving
// every other field (proposal + operator + OperatorSet) exactly as it was.
func (p ChannelPolicy) WithApplied(applied []AppliedRelaxation) ChannelPolicy {
	p.Applied = applied
	return p
}

// mergeRules composes rules by provenance (§8.2): keep every existing rule that is NOT
// LLM-sourced (operator-authored, or unknown "" — preserved fail-safe), then append the
// fresh proposal's rules (which groundRules stamps RuleSourceLLM). So a refine replaces the
// LLM rules and never drops a hand-authored one.
func mergeRules(existing, incoming []SchedulingRule) []SchedulingRule {
	out := make([]SchedulingRule, 0, len(existing)+len(incoming))
	for _, r := range existing {
		if !r.Source.isLLM() {
			out = append(out, r)
		}
	}
	out = append(out, incoming...)
	if len(out) == 0 {
		return nil // keep the zero value nil, not an empty slice (round-trip stability)
	}
	return out
}

// tighterCeiling returns the stricter (lower on the ladder) of two ceilings. "" (no
// ceiling) is the loosest, so any mapped ceiling wins over it. A merge may TIGHTEN a
// ceiling but never loosen one (§4).
func tighterCeiling(a, b Rating) Rating {
	ra, aok := a.Rank()
	rb, bok := b.Rank()
	switch {
	case !aok:
		return b // a is unrated/none → b is at least as strict
	case !bok:
		return a
	case rb < ra:
		return b
	default:
		return a
	}
}

func pathSet(paths []string) map[string]bool {
	s := make(map[string]bool, len(paths))
	for _, p := range paths {
		s[p] = true
	}
	return s
}

func sortedPaths(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
