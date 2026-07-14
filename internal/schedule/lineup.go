package schedule

import (
	"math/rand"

	"github.com/mantonx/loomarr/internal/provision"
)

// DesiredLineup is the ordered set of Slots a channel *should* have right now,
// computed from its approved lineup + current availability (§9). It is the
// "desired" half of desired-vs-actual reconciliation: the reconcile engine diffs
// it against Tunarr's actual channel state and applies the minimal calls.
type DesiredLineup struct {
	ChannelID string
	Slots     []Slot
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
func ComputeDesired(ch Channel, entries []LineupEntry, avail Availability, policy PendingPolicy) DesiredLineup {
	// Resolve each entry to its program slot(s) FIRST — a movie → one slot, a
	// series → one slot per episode (§9 expansion) — then order the whole slot
	// list by strategy. Ordering after expansion means `shuffle` shuffles episodes
	// with everything else, and `sequential` keeps them in season/episode order.
	slots := make([]Slot, 0, len(entries))
	for _, e := range entries {
		slots = append(slots, resolveEntry(e, avail, policy)...)
	}
	return DesiredLineup{ChannelID: ch.ID, Slots: orderSlots(ch, slots)}
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

// orderSlots applies the channel's strategy to the desired slot order (§9). It
// runs AFTER series expansion, so `shuffle` shuffles episodes together with
// everything else and `sequential` preserves season/episode (and lineup) order.
// Copies rather than mutating the caller's slice.
func orderSlots(ch Channel, slots []Slot) []Slot {
	out := make([]Slot, len(slots))
	copy(out, slots)

	switch ch.Strategy {
	case Shuffle:
		// Deterministic permutation from the channel's seed (§9/§10). rand with
		// a fixed source is reproducible across runs and platforms.
		r := rand.New(rand.NewSource(ch.Shuffle.Seed))
		r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	case Sequential, TimeSlot:
		// Keep resolved order. TimeSlot block mapping happens at push time.
	default:
		// Unknown strategy: preserve order (Validate rejects these upstream, but
		// ComputeDesired must never panic on bad data).
	}
	return out
}
