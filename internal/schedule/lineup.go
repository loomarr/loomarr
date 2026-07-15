package schedule

import (
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// DesiredLineup is the ordered set of Slots a channel *should* have right now,
// computed from its approved lineup + current availability (§9). It is the
// "desired" half of desired-vs-actual reconciliation: the reconcile engine diffs
// it against Tunarr's actual channel state and applies the minimal calls.
type DesiredLineup struct {
	ChannelID string
	Slots     []Slot
	// Excluded reports what the audience/scope hard filters dropped (§4), surfaced
	// at reconcile + proposal so a metadata gap is visible. Empty when nothing was
	// filtered (or no policy is set).
	Excluded ExclusionReport
	// Applied records the relaxation-ladder steps taken this computation (§7),
	// written back onto the channel's policy by the reconcile engine and surfaced in
	// the UI. Empty when the eligible pool satisfied the policy with no relaxation.
	Applied []AppliedRelaxation
}

// ProgramCount returns how many slots are real playable programs (§9: a channel
// with zero programs must still be live via filler/flex — never dead air).
func (d DesiredLineup) ProgramCount() int {
	n := 0
	for _, s := range d.Slots {
		if s.IsProgram() {
			n++
		}
	}
	return n
}

// Availability answers, for a provisioning Key, whether the title is available
// and (if so) its library item id. The reconcile engine backs this with the
// store/library; pure tests back it with a map. Mirrors §9's "compute desired
// from the approved lineup + live availability".
type Availability interface {
	// Resolve returns (libraryItemID, durationMs, true) if key is available now,
	// else ("", 0, false). durationMs is the program runtime from the library (§9,
	// §10 RunTimeTicks) — a program slot needs it (Tunarr rejects duration ≤ 0).
	// It must be side-effect free from the caller's view.
	//
	// For a SERIES key, Resolve returns available=false: a show isn't directly
	// playable (no single item/runtime). Series resolve via ResolveEpisodes.
	Resolve(key provision.Key) (libraryItemID string, durationMs int64, available bool)

	// ResolveEpisodes expands a SERIES key into its episode programs, in
	// season/episode order (§9 series expansion). Returns (nil, false) for a
	// non-series key or a series with no playable episodes yet (→ pending slot).
	// Each episode carries its own library item id + duration. Side-effect free.
	ResolveEpisodes(key provision.Key) (episodes []ResolvedProgram, available bool)
}

// ResolvedProgram is one concrete playable program (a movie or a single episode)
// with the fields a program Slot needs. Used by series expansion (§9). Season is
// the episode's season number (0 for a movie / unknown), used to apply a lineup
// entry's optional season range.
type ResolvedProgram struct {
	LibraryItemID string
	Title         string
	DurationMs    int64
	Season        int
}

// PendingPolicy is what to place where a lineup entry's title is not yet
// available (§9 pending-slot policy). Default is pod-fill.
type PendingPolicy string

const (
	// PodFill: fill the gap with matched filler so the timeline is continuous
	// (§9 default — "never dead air").
	PodFill PendingPolicy = "pod_fill"
	// ComingSoon: leave an explicit pending slot (a "coming soon" interstitial
	// card is a config alternative, §9). The slot stays SlotPending.
	ComingSoon PendingPolicy = "coming_soon"
)

// LineupEntry is one item of a channel's *approved* lineup (from a proposal,
// §8): the intent-level "this should play here". Availability decides whether it
// becomes a program slot or a pending slot this reconcile.
type LineupEntry struct {
	Key        provision.Key // provisioning key of the intended title
	Title      string        // display label
	DurationMs int64         // known runtime, if any (0 = unknown)
	// SeasonMin/SeasonMax optionally constrain a SERIES entry to a season range
	// (inclusive; 0 = unbounded on that end) — an intent-level filter such as
	// "old-school Simpsons" (§9 series expansion). Ignored for movies.
	SeasonMin int
	SeasonMax int
	// Policy-enforcement metadata, stamped from the grounded ProposalItem at
	// channel-create time (programming-design §4): the audience filter, era/genre
	// scope, and runtime cap read these off the entry so enforcement stays a pure
	// entry-set filter with no per-reconcile library I/O. For a series these are the
	// SERIES' values (v1 series-level ceiling; episodes carry no per-episode rating).
	OfficialRating Rating   // media-server content rating, normalized at enforcement
	Genres         []string // for genre scope + seasonal keyword matching
	Year           int      // for era scope + seasonal windowing
	RuntimeSec     int      // for the runtimeMax cap; 0 = unknown (not filtered on runtime)
}

// inSeasonRange reports whether an episode season falls within the entry's
// optional [SeasonMin, SeasonMax] (0 = unbounded on that end).
func (e LineupEntry) inSeasonRange(season int) bool {
	if e.SeasonMin > 0 && season < e.SeasonMin {
		return false
	}
	if e.SeasonMax > 0 && season > e.SeasonMax {
		return false
	}
	return true
}

// ComputeDesired turns an approved lineup + current availability into a
// DesiredLineup under a strategy and pending policy (§9). It is PURE: same
// inputs (including the channel's shuffle seed) always yield the same slots, so
// the reconcile engine's output is reproducible and tests need no mock Tunarr.
//
// Ordering:
//   - Sequential: entries stay in approved order.
//   - Shuffle: entries are permuted by a seed derived from the channel (§9/§10
//     determinism) — the same channel reshuffles identically every reconcile,
//     so a live channel's guide is stable, not re-randomized on every sweep.
//   - TimeSlot: block assignment is applied by the caller's block plan; here we
//     keep approved order and let the reconcile layer map onto Tunarr time
//     slots. (v1: treated as Sequential for slot derivation; block math is
//     applied at push time.)
//
// Availability mapping (per entry):
//   - available            → SlotProgram (LibraryItemID set)
//   - not available, PodFill → SlotFiller (pod-fill placeholder; §10 fills it)
//   - not available, ComingSoon → SlotPending (explicit gap)
//
// ComputeDesired is the policy-free entry point (an empty ChannelPolicy + zero
// clock): existing callers and tests get byte-identical behavior to the old blind
// ordering. Reconcile calls ComputeDesiredAt with the channel's policy + a clock.
func ComputeDesired(ch Channel, entries []LineupEntry, avail Availability, policy PendingPolicy) DesiredLineup {
	return ComputeDesiredAt(ch, entries, avail, policy, ChannelPolicy{}, time.Time{})
}

// ComputeDesiredAt is the ChannelPolicy-aware desired-lineup builder (programming-
// design §3–§7). It is PURE — same inputs (channel seed + policy + clock) always
// yield the same slots — so reconcile output is reproducible and tests need no mock
// Tunarr. `now` drives seasonality (§6); a zero `now` disables it (seasonal mode
// treated as off), which is why the policy-free wrapper passes time.Time{}.
//
// Pipeline: filter entries (scope + audience §4, never-relaxed) → seasonal bench
// (§6) → resolve each entry to slot(s) → policy-aware slotting (ordering §5 +
// separation-with-wrap §3) → relaxation ladder on shortfall (§7) → interleave
// breaks (§10, unchanged). The channel's Strategy supplies the ordering when the
// policy omits it (a policy-less channel keeps its existing behavior).
func ComputeDesiredAt(ch Channel, entries []LineupEntry, avail Availability, pending PendingPolicy, policy ChannelPolicy, now time.Time) DesiredLineup {
	rp := policy.Resolved(ch.Strategy, singleSeriesEntries(entries))

	// Hard filters first (§4 audience fail-closed + scope) — the never-relaxed gate.
	eligible, report := filterEntries(entries, rp)
	// Seasonal bench/boost (§6): out-of-window seasonal items are benched (removed);
	// in-window ones are marked for a scheduling boost. A zero clock ⇒ no-op.
	eligible, seasonalReport := applySeasonal(eligible, rp, now)
	report.merge(seasonalReport)

	// Resolve each eligible entry to its program slot(s) — movie → one, series → one
	// per in-range episode (§9 expansion).
	slots := make([]Slot, 0, len(eligible))
	for _, e := range eligible {
		slots = append(slots, resolveEntry(e, avail, pending)...)
	}

	// Policy-aware ordering + separation (§3/§5), then the relaxation ladder (§7)
	// records any windows it had to loosen to fill the cycle.
	ordered, applied := slotWithRelaxation(slots, rp, ch.Shuffle.Seed)

	return DesiredLineup{
		ChannelID: ch.ID,
		Slots:     interleaveBreaks(ch, ordered),
		Excluded:  report,
		Applied:   applied,
	}
}

// breakGapMs is the placeholder duration for an inserted commercial break — the
// pod assembler sizes the actual pod to this gap (§10 default 2-minute break).
const breakGapMs = 120_000

// interleaveBreaks inserts SlotFiller break gaps between program slots at the
// channel's BreaksPerHour density (§10). Runtime-aware and snapped to program
// boundaries (Tunarr only breaks between programs): walk the slots accumulating
// program runtime, and after a program that pushes the running total past the
// next 60/BreaksPerHour-minute threshold, emit one break and reset. Never a
// trailing break (nothing after it to return from) and never two in a row.
func interleaveBreaks(ch Channel, slots []Slot) []Slot {
	if ch.BreaksPerHour <= 0 || len(slots) == 0 {
		return slots
	}
	thresholdMs := int64(60*60*1000) / int64(ch.BreaksPerHour) // ms of runtime per break
	out := make([]Slot, 0, len(slots)+len(slots)/2)
	var acc int64
	for i, s := range slots {
		out = append(out, s)
		if !s.IsProgram() {
			continue // only program runtime counts toward the break cadence
		}
		acc += s.DurationMs
		// A break only makes sense between two programs — not after the last slot.
		hasLater := false
		for _, n := range slots[i+1:] {
			if n.IsProgram() {
				hasLater = true
				break
			}
		}
		if acc >= thresholdMs && hasLater {
			out = append(out, Slot{Kind: SlotFiller, DurationMs: breakGapMs})
			acc = 0
		}
	}
	return out
}

// resolveEntry turns one approved lineup entry into its desired slot(s).
func resolveEntry(e LineupEntry, avail Availability, policy PendingPolicy) []Slot {
	// A series expands into its episodes (each a program slot).
	if e.Key.IsSeries() {
		if eps, ok := avail.ResolveEpisodes(e.Key); ok && len(eps) > 0 {
			out := make([]Slot, 0, len(eps))
			for _, ep := range eps {
				if !e.inSeasonRange(ep.Season) {
					continue // outside the entry's season range (e.g. "old-school" only)
				}
				out = append(out, Slot{
					Kind:          SlotProgram,
					Key:           e.Key,
					LibraryItemID: ep.LibraryItemID,
					Title:         ep.Title,
					DurationMs:    ep.DurationMs,
				})
			}
			if len(out) > 0 {
				return out
			}
			// The range matched no in-library episodes yet → pending (like an
			// unavailable series), not an empty channel.
		}
		// Series with no playable (in-range) episodes yet → a single placeholder.
		return []Slot{pendingSlot(e, policy)}
	}

	// A movie (or any non-series) resolves to a single program slot.
	if itemID, durationMs, ok := avail.Resolve(e.Key); ok {
		if durationMs == 0 {
			durationMs = e.DurationMs // fall back to the entry's own duration
		}
		return []Slot{{
			Kind:          SlotProgram,
			Key:           e.Key,
			LibraryItemID: itemID,
			Title:         e.Title,
			DurationMs:    durationMs,
		}}
	}
	return []Slot{pendingSlot(e, policy)}
}
