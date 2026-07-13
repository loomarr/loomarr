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
	// Resolve returns (libraryItemID, true) if key is available now, else
	// ("", false). It must be side-effect free from the caller's view.
	Resolve(key provision.Key) (libraryItemID string, available bool)
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
	ordered := orderEntries(ch, entries)

	slots := make([]Slot, 0, len(ordered))
	for _, e := range ordered {
		if itemID, ok := avail.Resolve(e.Key); ok {
			slots = append(slots, Slot{
				Kind:          SlotProgram,
				Key:           e.Key,
				LibraryItemID: itemID,
				Title:         e.Title,
				DurationMs:    e.DurationMs,
			})
			continue
		}
		slots = append(slots, pendingSlot(e, policy))
	}

	return DesiredLineup{ChannelID: ch.ID, Slots: slots}
}

// orderEntries applies the channel's strategy to the approved lineup order. It
// copies rather than mutating the caller's slice.
func orderEntries(ch Channel, entries []LineupEntry) []LineupEntry {
	out := make([]LineupEntry, len(entries))
	copy(out, entries)

	switch ch.Strategy {
	case Shuffle:
		// Deterministic permutation from the channel's seed (§9/§10). rand with
		// a fixed source is reproducible across runs and platforms.
		r := rand.New(rand.NewSource(ch.Shuffle.Seed))
		r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	case Sequential, TimeSlot:
		// Keep approved order. TimeSlot block mapping happens at push time.
	default:
		// Unknown strategy: preserve order (Validate rejects these upstream, but
		// ComputeDesired must never panic on bad data).
	}
	return out
}
