// Package schedule is the scheduler domain (design §9): the Channel identity,
// the DesiredLineup / Slot model, and the *pure* computation that turns an
// approved lineup plus live availability into ordered desired programming. It is
// I/O-free — no store, no Tunarr client, no clock beyond what callers pass in —
// so strategy math (sequential / shuffle / time-slot) and the pending-slot
// policy are exhaustively unit-testable with an explicit seed. The reconcile
// engine (reconcile.go's callers, Phase 10) and the Programmer adapter
// (internal/programmer) supply the outside world; this package only *decides
// what should play*.
package schedule

import (
	"fmt"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// Strategy is how a channel's available programs are ordered onto the timeline
// (§9 scheduling strategies). It maps onto Tunarr's programming model.
type Strategy string

const (
	// Sequential plays programs in lineup order (e.g. a series in episode
	// order). Stable: the same available set always yields the same order.
	Sequential Strategy = "sequential"
	// Shuffle randomizes rotation. Deterministic under an explicit seed (§10
	// determinism discipline) so tests reproduce exactly.
	Shuffle Strategy = "shuffle"
	// TimeSlot pins programs to wall-clock blocks (cartoons AM, movies PM). The
	// block plan lives on the DesiredLineup; here it selects which slots feed
	// which block. Wall-clock semantics per §9 (TZ from the container).
	TimeSlot Strategy = "time_slot"
)

// Valid reports whether s is a known strategy.
func (s Strategy) Valid() bool {
	switch s {
	case Sequential, Shuffle, TimeSlot:
		return true
	}
	return false
}

// ChannelStatus is the lifecycle/health of a Loomarr-managed channel as shown on
// the Channels view (§12). It is Loomarr's own status, distinct from Tunarr's.
type ChannelStatus string

const (
	// StatusBuilding: created, initial reconcile has not yet converged on the
	// selected playout backend.
	StatusBuilding ChannelStatus = "building"
	// StatusLive: reconciled to the selected playout backend and ready to play
	// (possibly with pending slots still backfilling).
	StatusLive ChannelStatus = "live"
	// StatusDrifted: the periodic sweep found a scheduled program missing from
	// the library and substituted it (§9 slot revalidation) — surfaced so the
	// operator knows the lineup changed under them.
	StatusDrifted ChannelStatus = "drifted"
	// StatusEmpty: the lineup is non-empty but every entry was filtered out, so the
	// channel has NOTHING to air. Distinct from `building` (which is on its way) and
	// from `live` (which is playing).
	//
	// ⚠ This exists because the alternative was silence. `statusFor` used to return
	// `live` without ever looking at the deck, so a channel could report healthy while
	// broadcasting nothing — which is exactly how a seasonal-bench bug hid: six titles
	// on the lineup, `desired_json` literally `[]`, status `live`, and an empty grid
	// as the operator's only symptom. The reason is already computed (the
	// ExclusionReport names each benched entry and why); this status is what makes it
	// worth surfacing.
	StatusEmpty ChannelStatus = "empty"
	// StatusDetached: Loomarr no longer manages this channel (soft-deleted; a
	// retained external projection may still exist unless purge=true).
	StatusDetached ChannelStatus = "detached"
	// StatusPaused: the operator deliberately took the channel off the sweep — a
	// resumable "off air, but keep it" state, distinct from detached (which is "no
	// longer managed"). Like detached it is never auto-reconciled (the sweep skips
	// it), but unlike detached it is a normal, non-error state that resumes to
	// `building` via a PATCH. A retained Tunarr projection keeps its last lineup.
	StatusPaused ChannelStatus = "paused"
)

// Reconcilable reports whether automatic or direct reconciliation may manage a channel.
// Paused and detached are the two explicit lifecycle opt-outs; every other status remains
// active so building, empty, live, and drifted channels can converge or recover.
func (s ChannelStatus) Reconcilable() bool {
	return s != StatusPaused && s != StatusDetached
}

// Channel is Loomarr's backend-neutral channel definition (§9 scheduler domain).
// ID is Loomarr-owned. TunarrID is the optional server-assigned identifier of a
// Tunarr projection (Tunarr ignores client-supplied ids), so internal channels and
// channels not yet projected to Tunarr leave it empty.
type Channel struct {
	ID        string   // Loomarr id (uuid-like, assigned on create)
	IntentRef string   // proposal/intent this channel came from (§8); "" for a hand-made channel
	Name      string   // display name
	Number    int      // channel number (guide position)
	Group     string   // channel grouping in the guide
	Logo      string   // optional channel icon URL
	Strategy  Strategy // ordering strategy
	// BreaksPerHour is the commercial-break density (§10): how many ad breaks per
	// hour of program runtime the scheduler interleaves between programs (0 = none).
	// Sourced from FILLER_BREAKS_PER_HOUR at channel build; empty breaks fill with
	// matched pods at reconcile (fillPods), else stay flex (never dead air).
	BreaksPerHour int
	// BreakDurationMs is the target duration of each inserted filler slot. Resolved from the
	// per-channel override or live global setting before this pure scheduler runs.
	BreakDurationMs int64
	// LastAired is when each key last aired on THIS channel (§3.1) — the recency signal
	// placement biases on, loaded from the airings table by the caller.
	//
	// Observed state rather than configuration: nothing authors it, it has no ChannelPolicy
	// counterpart, and an empty map is always valid (a fresh channel, or a store that could
	// not answer) — placement then behaves exactly as it did before recency existed.
	LastAired map[provision.Key]time.Time
	// DefaultWindow is the global rolling-window horizon (§6.5, sched.window_hours,
	// default 24h) reconcile sets from settings before ComputeDesiredAt — the pure
	// schedule package can't read settings, so this transient field carries the default
	// the way BreaksPerHour does. A channel/rule Window overrides it; 0 here = the whole
	// run (the policy-free path leaves it 0, preserving today's un-windowed behavior).
	DefaultWindow time.Duration
	FillerRef     string        // ref to the channel's filler list (§10); "" = none yet
	TunarrID      string        // retained Tunarr projection id; "" until first-ever successful projection
	Status        ChannelStatus // Loomarr-side status
	Shuffle       ShuffleParams // shuffle seed material (used only when Strategy==Shuffle)
	UpdatedAt     int64         // epoch seconds (store stamps this; §5 epoch-BIGINT convention)
}

// ShuffleParams carries the deterministic seed material for Shuffle (§9/§10). The
// seed is derived from (channel + window) so a given channel reproduces the same
// order in tests and across restarts — never Math.random-style nondeterminism.
type ShuffleParams struct {
	Seed int64
}

// Validate checks the invariants a Channel must satisfy before it can be
// reconciled. It does not touch I/O.
func (c Channel) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("channel: empty id")
	}
	if c.Name == "" {
		return fmt.Errorf("channel %s: empty name", c.ID)
	}
	if c.Number <= 0 {
		return fmt.Errorf("channel %s: number must be positive, got %d", c.ID, c.Number)
	}
	if !c.Strategy.Valid() {
		return fmt.Errorf("channel %s: invalid strategy %q", c.ID, c.Strategy)
	}
	return nil
}

// SlotKind classifies what a Slot resolves to (§9): a real program, a slot still
// awaiting the provisioner, or filler/flex padding.
type SlotKind string

const (
	// SlotProgram: a library item that is available now (LibraryItemID set).
	SlotProgram SlotKind = "program"
	// SlotPending: an acquisition still in flight; the provisioner will emit
	// `available` later. Backfilled in place (§9 stable placement).
	SlotPending SlotKind = "pending"
	// SlotFiller: matched filler/bumper/pod content (§10). Never counts as a
	// "program" in the lineup (§19 filler-never-a-program).
	SlotFiller SlotKind = "filler"
	// SlotFlex: Tunarr flex (dead-time padding) — the last-resort "never dead
	// air" fill when even filler is unavailable.
	SlotFlex SlotKind = "flex"
)

// Slot is one entry in a channel's ordered lineup (§9). It references content by
// *external* id (Key) so a not-yet-available title still has a stable slot; once
// available, LibraryItemID is the Tunarr/library item to play.
type Slot struct {
	Kind          SlotKind
	Key           provision.Key // provisioning key for program/pending slots; "" for filler/flex
	LibraryItemID string        // media-server/Tunarr item id, set once available
	Title         string        // display label (for the guide / logs)
	DurationMs    int64         // program duration; 0 when unknown (pending) or flex-elastic
	// PartGroup ties the slots of a multi-part story together (§5): all parts of one
	// two-parter share a non-empty PartGroup, and PartIndex is their play order within it
	// (1, 2, …). "" = a standalone program. The ordering layer treats a group as one
	// atomic unit — parts stay adjacent + in-order, and the rolling window keeps them whole.
	PartGroup string
	PartIndex int
	// Season / Episode are the series episode's numbers (from the media server's
	// IndexNumber), carried onto the slot purely for DISPLAY — the preview surfaces
	// "Bluey · S1E5". 0 = a movie or unknown. They do not affect ordering or identity
	// (the Key + PartGroup already govern that); a slot with the same Key airing twice in
	// a marathon simply repeats its S/E.
	Season  int
	Episode int
	// SeriesTitle is the SHOW's name for an episode slot ("The Simpsons"), while Title holds
	// the EPISODE's name ("Life on the Fast Lane"). "" for a movie.
	//
	// Carried for display only, like Season/Episode. It exists because XMLTV splits the two —
	// `<title>` is the series and `<sub-title>` is the episode — and without it a guide lists
	// every episode as an unrelated programme: a media server groups and searches by `<title>`,
	// so "Life on the Fast Lane" appears with no indication it is The Simpsons.
	SeriesTitle string
}

// IsProgram reports whether this slot is a real, playable program (not pending,
// filler, or flex). The scheduler counts these to decide if a channel has any
// real content yet.
func (s Slot) IsProgram() bool { return s.Kind == SlotProgram }
