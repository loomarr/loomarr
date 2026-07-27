package schedule

import (
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// This file is the ChannelPolicy ENFORCEMENT half (programming-design §3–§5): the
// deterministic filters + constraint-aware slotting that turn an approved lineup
// into desired programming under a policy. The suggester only ever proposes rule
// VALUES; nothing here trusts the model — every rule is a machine check.
//
// Pipeline (called from ComputeDesiredAt):
//
//	filterEntries (scope + audience §4 → eligible entries + ExclusionReport)
//	  → resolveEntry each (expand series → episode slots; unchanged)
//	  → slotByPolicy (ordering §5 + separation-with-wrap §3)
//	  → interleaveBreaks (unchanged, downstream)

// ExclusionReport summarizes what the audience filter dropped (§4), surfaced at
// proposal review and reconcile so a metadata gap is visible before approval.
type ExclusionReport struct {
	OverCeiling int            `json:"overCeiling"` // items above the rating ceiling
	Unrated     int            `json:"unrated"`     // items excluded because unrated under a kids ceiling
	Items       []ExcludedItem `json:"items,omitempty"`
}

// ExcludedItem is one dropped entry, for the UI ("why isn't X on my channel").
type ExcludedItem struct {
	Key    provision.Key `json:"key"`
	Title  string        `json:"title"`
	Reason string        `json:"reason"` // "over_ceiling" | "unrated" | "out_of_scope" | "out_of_season"
}

// merge folds another report's items/counts into this one (used to combine the
// audience/scope filter report with the seasonal-bench report, §6).
func (r *ExclusionReport) merge(o ExclusionReport) {
	r.OverCeiling += o.OverCeiling
	r.Unrated += o.Unrated
	r.Items = append(r.Items, o.Items...)
}

// filterEntries applies the hard filters to the approved entries and returns the
// eligible set plus the exclusion report. Order: scope (era/genre/runtime) then
// audience (§4 fail-closed). Audience and explicit scope are the NEVER-relaxed
// filters (§7) — they run here, before the relaxation ladder ever sees the pool.
//
// It does NOT enforce the per-series Seasons window or the Series allowlist at the
// entry level for a series entry — those apply at episode expansion (resolveEntry
// already honors SeasonMin/Max) — but a Series allowlist, an Era, a Genre, and a
// RuntimeMax filter apply here.
func filterEntries(entries []LineupEntry, rp ResolvedPolicy) ([]LineupEntry, ExclusionReport) {
	var report ExclusionReport
	eligible := make([]LineupEntry, 0, len(entries))

	seriesAllow := seriesAllowSet(rp.Scope.Series)

	for _, e := range entries {
		// Explicit series allowlist (never relaxed): if set, keep only listed series
		// (and any non-series entry the channel also carries).
		if seriesAllow != nil && e.Key.IsSeries() {
			if _, ok := seriesAllow[e.Key]; !ok {
				report.add(e, "out_of_scope")
				continue
			}
		}
		// Era: filter by the entry's year when the entry has one. An entry with no
		// year (year 0) is NOT excluded on era — we don't guess (a series often lacks
		// a single production year); its episodes still carry the intent.
		if e.Year > 0 && !rp.Scope.Era.Contains(e.Year) {
			report.add(e, "out_of_scope")
			continue
		}
		// Genre include/exclude (§2). Exclude always wins; Include (when non-empty)
		// requires at least one match. An entry with no genres passes Include only if
		// Include is empty (we don't fabricate a match, but we also don't exclude on a
		// metadata gap for genre — genre is a taste filter, not a safety one).
		if !genreOK(e.Genres, rp.Scope.Genres) {
			report.add(e, "out_of_scope")
			continue
		}
		// Runtime cap (§2 "nothing over an hour"): only when both the cap and the
		// entry runtime are known.
		if rp.Scope.RuntimeMax > 0 && e.RuntimeSec > 0 && e.RuntimeSec > rp.Scope.RuntimeMax {
			report.add(e, "out_of_scope")
			continue
		}
		// Audience ceiling (§4) — the fail-closed safety filter, NEVER relaxed.
		if rp.Ceiling != "" {
			switch audienceVerdict(e.OfficialRating, rp.Ceiling, rp.Unrated) {
			case verdictOverCeiling:
				report.OverCeiling++
				report.add(e, "over_ceiling")
				continue
			case verdictUnratedExcluded:
				report.Unrated++
				report.add(e, "unrated")
				continue
			}
		}
		eligible = append(eligible, e)
	}
	return eligible, report
}

func (r *ExclusionReport) add(e LineupEntry, reason string) {
	r.Items = append(r.Items, ExcludedItem{Key: e.Key, Title: e.Title, Reason: reason})
}

// audienceVerdict decides an entry against the ceiling (§4). Called only when a
// ceiling is set.
type verdict int

const (
	verdictKeep verdict = iota
	verdictOverCeiling
	verdictUnratedExcluded
)

func audienceVerdict(raw Rating, ceiling Rating, unrated UnratedPolicy) verdict {
	r := normalizeRating(string(raw)) // raw may already be normalized (stamped) — idempotent
	if r.mapped() {
		if r.atOrBelow(ceiling) {
			return verdictKeep
		}
		return verdictOverCeiling
	}
	// Unrated / unmappable: fail-closed under an exclude policy (kids default).
	if unrated == UnratedExclude {
		return verdictUnratedExcluded
	}
	return verdictKeep
}

// seriesAllowSet builds a lookup of the explicit series allowlist, or nil when
// there is no allowlist (⇒ no series restriction).
func seriesAllowSet(series []provision.Key) map[provision.Key]struct{} {
	if len(series) == 0 {
		return nil
	}
	m := make(map[provision.Key]struct{}, len(series))
	for _, k := range series {
		m[k] = struct{}{}
	}
	return m
}

// genreOK applies the include/exclude genre filter (§2). Exclude wins; a non-empty
// Include requires a match. Matching is case-insensitive.
func genreOK(genres []string, f GenreFilter) bool {
	if len(f.Exclude) > 0 {
		for _, g := range genres {
			for _, x := range f.Exclude {
				if strings.EqualFold(g, x) {
					return false
				}
			}
		}
	}
	if len(f.Include) > 0 {
		for _, g := range genres {
			for _, in := range f.Include {
				if strings.EqualFold(g, in) {
					return true
				}
			}
		}
		return false // Include set but nothing matched
	}
	return true
}

// singleSeriesEntries reports whether the eligible set is exactly one series and no
// movies (§2 single-series auto-relax input). Computed on entries before expansion.
func singleSeriesEntries(entries []LineupEntry) bool {
	seriesKeys := map[provision.Key]struct{}{}
	for _, e := range entries {
		if !e.Key.IsSeries() {
			return false // a movie present → mixed channel
		}
		seriesKeys[e.Key] = struct{}{}
	}
	return len(seriesKeys) == 1
}

// slotWithRelaxation orders the slots under the policy and, if the eligible pool
// can't satisfy the separation window, descends the relaxation ladder (§7),
// recording each applied step. Tier 3 wires the ordering; the ladder body lands in
// relax.go (Tier 5) — until then it applies the policy once and records nothing.
func slotWithRelaxation(slots []Slot, rp ResolvedPolicy, seed int64) ([]Slot, []AppliedRelaxation) {
	return relaxAndSlot(slots, rp, seed)
}

// slotByPolicy orders the resolved program slots under the resolved ordering mode
// and separation rules (§3/§5). It is the constraint-aware replacement for the old
// blind orderSlots. Deterministic: all randomness derives from `seed`.
//
// Non-program slots (pending placeholders) are appended after the ordered programs
// — they carry no separation semantics and must not be shuffled into the middle of
// a syndication deck.
func slotByPolicy(slots []Slot, rp ResolvedPolicy, seed int64) []Slot {
	programs, others := partitionPrograms(slots)
	if len(programs) == 0 {
		return slots // nothing to order (all pending/filler) — preserve as-is
	}

	// Recency bias (§3.1): among the shuffled deck, float what has NOT been on recently toward
	// the front. Applied BEFORE separation repair so the §3 constraint engine still has the
	// final say — recency is a preference, never a rule it can override.
	//
	// SEQUENTIAL is deliberately exempt: that mode means "play this show in episode order", and
	// re-ordering it by recency would destroy the one property it exists to provide.
	var ordered []Slot
	switch rp.Ordering {
	case OrderSequential:
		ordered = append([]Slot(nil), programs...) // keep resolved (season/episode + lineup) order
	case OrderShuffle:
		ordered = byRecency(seededShuffle(programs, seed), rp.LastAired)
		ordered = separationRepair(ordered, rp)
	case OrderSyndication:
		ordered = syndicationDeck(byRecency(programs, rp.LastAired), rp, seed)
	default:
		ordered = append([]Slot(nil), programs...)
	}

	// Heal the loop seam (§3): Tunarr replays the list as a cycle, so the last→first
	// adjacency must honor separation too. A bounded rotation is enough to fix the
	// common seam violation without disturbing the interior order.
	ordered = healSeam(ordered, rp)

	return append(ordered, others...)
}

// partitionPrograms splits program slots (which carry separation semantics) from
// pending/filler/flex slots (which don't), preserving each sub-order.
func partitionPrograms(slots []Slot) (programs, others []Slot) {
	for _, s := range slots {
		if s.IsProgram() {
			programs = append(programs, s)
		} else {
			others = append(others, s)
		}
	}
	return programs, others
}

// seededShuffle returns a deterministic permutation of slots (same as the old
// orderSlots Shuffle branch — the empty-policy shuffle channel reproduces exactly).
func seededShuffle(slots []Slot, seed int64) []Slot {
	out := append([]Slot(nil), slots...)
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// seriesKeyOf returns the series identity a slot belongs to for separation
// purposes: the slot's Key (a series' episodes all share the series Key). A movie's
// Key is its own; blockMax/seriesMinGap then naturally apply per-title.
func seriesKeyOf(s Slot) provision.Key { return s.Key }

// byRecency floats least-recently-aired programmes toward the front of the deck (§3.1).
//
// # Why this is a SOFT signal
//
// It reorders a deck that is already valid; it never rejects a placement. A hard "no repeat
// within N days" is arithmetically unsatisfiable on a real channel — a 24h day consumes ~13
// films, so a week without repeats needs ~168h of content and the dev channel has ~62h — so a
// constraint would relax on every single run and teach operators to ignore the ladder (§7). This
// changes WHICH title fills a slot when several are equally valid, and nothing else.
//
// # Determinism
//
// STABLE sort over the already-seeded order, so equal-recency slots keep their shuffled
// positions: same pool + policy + seed + history ⇒ same deck (§3). A title with no history sorts
// FIRST — never aired is maximally "not recent", which is what puts new acquisitions on screen
// promptly.
//
// Empty history (a fresh install, a channel that has never aired, a store that could not answer)
// leaves the order untouched, so placement degrades to exactly its pre-§3.1 behaviour.
func byRecency(slots []Slot, lastAired map[provision.Key]time.Time) []Slot {
	if len(lastAired) == 0 || len(slots) <= 1 {
		return slots
	}
	out := append([]Slot(nil), slots...)
	sort.SliceStable(out, func(i, j int) bool {
		ti, oki := lastAired[out[i].Key]
		tj, okj := lastAired[out[j].Key]
		switch {
		case !oki && !okj:
			return false // both never aired — keep the seeded order
		case !oki:
			return true // i has never aired; it goes first
		case !okj:
			return false
		default:
			return ti.Before(tj) // older airing first
		}
	})
	return out
}
