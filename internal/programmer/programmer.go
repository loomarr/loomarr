// Package programmer is the Programmer boundary (design §6/§9): the port the
// scheduler drives to make a Loomarr channel real, plus its only v1
// implementation, a thin hand-written Tunarr client (§6: "hand-write a thin
// client against only the endpoints we use" — not codegen against Tunarr's
// churny pre-1.0 spec). The interface is abstracted so a future ErsatzTV/dizqueTV
// target is possible (§9); nothing above this package knows it's talking to
// Tunarr.
package programmer

import (
	"context"

	"github.com/mantonx/loomarr/internal/schedule"
)

// ChannelSpec is the desired Tunarr channel definition the scheduler wants to
// exist (§9). It is the Programmer-facing projection of a store.Channel — no
// Loomarr-internal fields, only what Tunarr needs.
type ChannelSpec struct {
	// TunarrID is the server-assigned channel id, "" on first create. The
	// adapter reads the real id from the create response (Phase-0 finding:
	// Tunarr ignores client-supplied ids) and returns it; the scheduler persists
	// it and passes it back on every subsequent call.
	TunarrID string
	Number   int
	Name     string
	Group    string // Tunarr groupTitle
	Logo     string // icon path; may be ""
	// StartTime is the channel's loop anchor in epoch MILLISECONDS — the moment its
	// looping lineup begins. 0 = "not set, stamp now on create". On an UPDATE the caller
	// threads Tunarr's EXISTING startTime here so the anchor is preserved (a fresh
	// time.Now() every reconcile would reset the loop origin, making the channel jump
	// back to the start of its lineup each sweep). Left 0 anchored the loop at epoch 0
	// (1970), which surfaced in the guide as a ~1960 programming start.
	StartTime int64
}

// Programmer is the port the scheduler reconciles against (§9). All methods are
// idempotent where the underlying API allows and safe to re-run (desired-vs-
// actual). Implementations must be resilient (§6 client defaults: per-call
// timeout, no unbounded retries in the hot path).
type Programmer interface {
	// EnsureChannel creates the channel if TunarrID is "", else updates it to
	// match spec. Returns the (server-assigned) TunarrID — the caller MUST
	// persist it, since a create ignores any client id (Phase-0 finding).
	EnsureChannel(ctx context.Context, spec ChannelSpec) (tunarrID string, err error)

	// GetChannel fetches a channel by its Tunarr id; returns ok=false if the
	// channel doesn't exist (so the reconcile knows to recreate). Used to diff
	// desired-vs-actual and detect out-of-band deletion.
	GetChannel(ctx context.Context, tunarrID string) (ActualChannel, bool, error)

	// ListChannels returns EVERY channel Tunarr has — including ones Loomarr did not create.
	//
	// ⚠ Read-only, and it must stay that way: §9's "channels Loomarr didn't create are never
	// touched" is unchanged. This exists so channel NUMBERING can see the whole space rather than
	// only Loomarr's own store, which is how a number Tunarr already used got assigned and made
	// the create fail forever (§9 V54).
	ListChannels(ctx context.Context) ([]ActualChannel, error)

	// SetLineup replaces the channel's programming with the desired slots
	// (§9). The adapter translates []schedule.Slot into Tunarr's manual-lineup
	// envelope. Idempotent: pushing the same slots twice yields the same
	// programming.
	SetLineup(ctx context.Context, tunarrID string, slots []schedule.Slot) error

	// GetLineup reads back the channel's current programming as slots, for the
	// desired-vs-actual diff. An unprogrammed channel returns an empty slice, not
	// an error (the adapter absorbs Tunarr's 400-on-empty-lineup quirk, Phase-0
	// finding 4).
	GetLineup(ctx context.Context, tunarrID string) ([]schedule.Slot, error)

	// DeleteChannel removes the Tunarr channel (used by DELETE /v1/channels?purge).
	// Deleting an already-absent channel is not an error (idempotent).
	DeleteChannel(ctx context.Context, tunarrID string) error

	// EnsureFillerList builds/updates the channel's Loomarr-owned Tunarr filler
	// list from the matched clip pool (identified by their Tunarr program uuids)
	// and attaches it to the channel (§10). Tunarr then plays those clips into the
	// flex gaps the scheduler leaves between programs. The adapter resolves each
	// uuid to the FULL program object Tunarr's filler-list API requires (a minimal
	// {id,duration} is rejected) from its own cached local-source index — so the
	// domain never carries raw Tunarr JSON. Idempotent and INTERNALLY no-op on an
	// unchanged program set: the filler-list contents are NOT part of the lineup
	// diff, so this method must avoid a redundant write itself (§9 "second
	// reconcile makes no writes"). An empty slice detaches the list (the channel
	// falls back to flex / the embedded bumper card — never dead air, §10).
	EnsureFillerList(ctx context.Context, tunarrID string, programIDs []string) error
}

// ActualChannel is the subset of Tunarr's channel we compare against desired
// during reconcile (§9 minimal-diff). Extend only with fields the diff needs.
type ActualChannel struct {
	TunarrID     string
	Number       int
	Name         string
	Group        string
	Logo         string
	ProgramCount int   // Tunarr's programCount; 0 = unprogrammed
	StartTime    int64 // loop anchor (epoch ms); read back so an update preserves it
}
