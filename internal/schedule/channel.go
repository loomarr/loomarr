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
	// StatusBuilding: created, initial reconcile not yet pushed to Tunarr.
	StatusBuilding ChannelStatus = "building"
	// StatusLive: reconciled to Tunarr and playing (possibly with pending slots
	// still backfilling).
	StatusLive ChannelStatus = "live"
	// StatusDrifted: the periodic sweep found a scheduled program missing from
	// the library and substituted it (§9 slot revalidation) — surfaced so the
	// operator knows the lineup changed under them.
	StatusDrifted ChannelStatus = "drifted"
	// StatusDetached: Loomarr no longer manages this channel (soft-deleted; the
	// Tunarr channel may still exist unless purge=true).
	StatusDetached ChannelStatus = "detached"
	// StatusPaused: the operator deliberately took the channel off the sweep — a
	// resumable "off air, but keep it" state, distinct from detached (which is "no
	// longer managed"). Like detached it is never auto-reconciled (the sweep skips
	// it), but unlike detached it is a normal, non-error state that resumes to
	// `building` via a PATCH. v1 leaves the Tunarr channel playing its last lineup.
	StatusPaused ChannelStatus = "paused"
)

// Channel is a Loomarr-managed Tunarr channel (§9 scheduler domain). Identity is
// Loomarr's own ID; TunarrID is the *server-assigned* id from Tunarr's create
// response (Phase-0 finding: Tunarr ignores client-supplied ids), empty until
// the first reconcile creates the Tunarr channel.
type Channel struct {
	ID        string   // Loomarr id (uuid-like, assigned on create)
	IntentRef string   // proposal/intent this channel came from (§8); "" for a hand-made channel
	Name      string   // display name / Tunarr channel name
	Number    int      // Tunarr channel number (guide position)
	Group     string   // Tunarr groupTitle (channel grouping in the guide)
	Logo      string   // icon URL (Tunarr channel.icon.path); optional
	Strategy  Strategy // ordering strategy
	// BreaksPerHour is the commercial-break density (§10): how many ad breaks per
	// hour of program runtime the scheduler interleaves between programs (0 = none).
	// Sourced from FILLER_BREAKS_PER_HOUR at channel build; empty breaks fill with
	// matched pods at reconcile (fillPods), else stay flex (never dead air).
	BreaksPerHour int
	// DefaultWindow is the global rolling-window horizon (§6.5, sched.window_hours,
	// default 24h) reconcile sets from settings before ComputeDesiredAt — the pure
	// schedule package can't read settings, so this transient field carries the default
	// the way BreaksPerHour does. A channel/rule Window overrides it; 0 here = the whole
	// run (the policy-free path leaves it 0, preserving today's un-windowed behavior).
	DefaultWindow time.Duration
	FillerRef     string        // ref to the channel's filler list (§10); "" = none yet
	TunarrID      string        // server-assigned Tunarr channel id; "" until first reconcile
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
}

// IsProgram reports whether this slot is a real, playable program (not pending,
// filler, or flex). The scheduler counts these to decide if a channel has any
// real content yet.
func (s Slot) IsProgram() bool { return s.Kind == SlotProgram }
