package schedule

import "github.com/mantonx/loomarr/internal/provision"

// Lineup application (programming-design §9). Every write to a channel's approved lineup
// goes through ApplyLineup, so "what changes the lineup" has ONE implementation with an
// explicit mode instead of four hand-rolled mutators. The manual-edit, refine, and
// auto-curate triggers are all callers with a mode (§8.2 "one primitive, three triggers");
// the reconcile metadata heal is the fourth. The Replace/Additive membership logic is pure
// (deterministic, §10); the only non-pure inputs — library availability for the additive
// prune, rating/collection lookups for the heal — are injected as callbacks in ApplyOpts, so
// store/TMDB I/O stays in the caller.

// LineupMode selects how ApplyLineup combines the current lineup with the incoming one.
type LineupMode int

const (
	// LineupReplace: the incoming lineup wins wholesale — an operator PATCH, or a human/
	// refine approval where a person decided (including to remove titles). With
	// opts.PreserveByKey, the rich scheduling metadata (duration/rating/runtime/collection,
	// and the season window when the incoming omits it) is carried forward from the matching
	// current entry: the read DTO is lossy, so a reorder must not silently wipe a season
	// scope or a healed rating (§7 PATCH contract).
	LineupReplace LineupMode = iota
	// LineupAdditive: unattended auto-curate (§8.2) — union the incoming picks onto the
	// existing lineup, dropping an existing title only when opts.Drop reports it clearly
	// off-intent (gone from the library, or explicitly excluded). A still-available title the
	// stochastic LLM merely didn't re-pick is KEPT — omission is not a decision to remove, so
	// re-curation never churns. Existing kept-order first, genuinely-new picks appended.
	LineupAdditive
	// LineupHeal: enrich the CURRENT lineup in place (opts.Enrich per entry) WITHOUT changing
	// membership or order — the reconcile filling empty ratings/collection ids (§389/§5).
	// The incoming argument is ignored.
	LineupHeal
)

// ApplyOpts carries the mode-specific behavior. The callbacks keep I/O out of this package.
type ApplyOpts struct {
	// PreserveByKey (LineupReplace only): carry rich metadata forward from the current entry
	// with the same key when the incoming entry left that field zero.
	PreserveByKey bool
	// Drop (LineupAdditive only): reports whether an existing entry NOT re-picked by the
	// incoming set is clearly off-intent and may be dropped. Injected so the store lookup
	// stays out of this pure function. Nil ⇒ nothing is dropped (a pure union).
	Drop func(LineupEntry) bool
	// Enrich (LineupHeal only): fills empty metadata on an entry in place (injected rating/
	// collection lookups). Nil ⇒ no-op.
	Enrich func(*LineupEntry)
}

// ApplyLineup is the single entry point for every lineup write (§9). It never touches the
// store or Tunarr — the caller persists the result and reconciles.
func ApplyLineup(current, incoming []LineupEntry, mode LineupMode, opts ApplyOpts) []LineupEntry {
	switch mode {
	case LineupAdditive:
		return applyAdditive(current, incoming, opts.Drop)
	case LineupHeal:
		return applyHeal(current, opts.Enrich)
	default: // LineupReplace
		if opts.PreserveByKey {
			return applyReplacePreserve(current, incoming)
		}
		return incoming
	}
}

// applyReplacePreserve replaces with `incoming` but carries each entry's rich scheduling
// metadata forward from the current entry of the same key (the lossy-DTO guard). Display
// fields (Title/Year/Genres) always come from `incoming`; a season window comes from
// `incoming` when it sets one, else from the current entry.
func applyReplacePreserve(current, incoming []LineupEntry) []LineupEntry {
	byKey := make(map[provision.Key]LineupEntry, len(current))
	for _, e := range current {
		byKey[e.Key] = e
	}
	out := make([]LineupEntry, len(incoming))
	for i, in := range incoming {
		if cur, ok := byKey[in.Key]; ok {
			out[i] = mergePreserve(cur, in)
		} else {
			out[i] = in
		}
	}
	return out
}

// mergePreserve overlays `in` (display fields + any season window it sets) onto the rich
// metadata of `cur`. Only fields `in` left zero are backfilled, so an explicit edit wins.
func mergePreserve(cur, in LineupEntry) LineupEntry {
	out := in
	if out.DurationMs == 0 {
		out.DurationMs = cur.DurationMs
	}
	if out.OfficialRating == "" {
		out.OfficialRating = cur.OfficialRating
	}
	if out.RuntimeSec == 0 {
		out.RuntimeSec = cur.RuntimeSec
	}
	if out.CollectionID == 0 {
		out.CollectionID = cur.CollectionID
	}
	if out.SeasonMin == 0 && out.SeasonMax == 0 {
		out.SeasonMin, out.SeasonMax = cur.SeasonMin, cur.SeasonMax
	}
	return out
}

// applyAdditive unions `fresh` onto `existing`: keep every existing entry except one that is
// (a) not re-picked by `fresh` AND (b) reported droppable, then append the genuinely-new
// picks. Order: existing-kept first (preserve the channel's shape), then new.
func applyAdditive(existing, fresh []LineupEntry, drop func(LineupEntry) bool) []LineupEntry {
	freshByKey := make(map[provision.Key]struct{}, len(fresh))
	for _, e := range fresh {
		freshByKey[e.Key] = struct{}{}
	}
	out := make([]LineupEntry, 0, len(existing)+len(fresh))
	kept := make(map[provision.Key]struct{}, len(existing))
	for _, e := range existing {
		if _, repicked := freshByKey[e.Key]; !repicked && drop != nil && drop(e) {
			continue // not re-picked AND clearly off-intent → drop
		}
		out = append(out, e)
		kept[e.Key] = struct{}{}
	}
	for _, e := range fresh {
		if _, dup := kept[e.Key]; dup {
			continue
		}
		out = append(out, e)
		kept[e.Key] = struct{}{}
	}
	return out
}

// applyHeal enriches each current entry in place without changing membership or order.
func applyHeal(current []LineupEntry, enrich func(*LineupEntry)) []LineupEntry {
	if enrich == nil {
		return current
	}
	for i := range current {
		enrich(&current[i])
	}
	return current
}
