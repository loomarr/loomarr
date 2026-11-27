package channels

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/activitystore"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
)

// ensureChannel creates or updates the Tunarr channel. On create, Tunarr assigns
// the id (Phase-0 finding 1) — EnsureChannel returns it; we must persist it.
// Handles out-of-band deletion: if we hold a TunarrID but the channel is gone,
// recreate it.
// Returns the Tunarr id, the number actually used, and whether Ensure created/recreated the
// remote row. The number may differ from the requested one when Tunarr already had a channel
// there (see below). The caller must persist both id and number in one checkpoint, or Loomarr's
// row and Tunarr disagree about what number the channel is on.
func (e *Engine) ensureChannel(ctx context.Context, localChannelID string, spec tunarr.ChannelSpec) (string, int, bool, error) {
	created := spec.TunarrID == ""
	if spec.TunarrID != "" {
		actual, ok, err := e.prog.GetChannel(ctx, spec.TunarrID)
		if err != nil {
			return "", spec.Number, false, fmt.Errorf("check channel %s: %w", spec.TunarrID, err)
		}
		if !ok {
			// The channel was deleted in Tunarr out of band; recreate it.
			spec.TunarrID = ""
			created = true
		} else {
			// Preserve Tunarr's existing loop anchor across the update PUT (§9) — without
			// this the channel's lineup would jump back to its start every reconcile. This
			// reuses the GET we already make here, so it costs no extra round-trip.
			spec.StartTime = actual.StartTime
		}
	}
	// ⚠ **A CREATE onto a number Tunarr already uses can never succeed, so move rather than retry
	// it forever** (§9 V54). Tunarr reports the collision as `500` with an EMPTY BODY — there is
	// nothing in the response to match on — so the occupancy is checked BEFORE the create instead
	// of interpreting the failure afterwards.
	//
	// ⚠ It renumbers LOOMARR'S channel and never touches the occupant. §9's "channels Loomarr
	// didn't create are never touched" is the rule this design is shaped around: after a database
	// reset Loomarr genuinely cannot tell its own orphan from a stranger's channel, so it must
	// assume stranger. Moving ourselves is the only self-healing option that cannot damage
	// somebody else's channel.
	//
	// The renumber is REPORTED, not silent: the number is what a viewer tunes to, so a channel
	// that quietly moved would be a worse surprise than the failure it replaces.
	if spec.TunarrID == "" {
		moved, err := e.freeNumberFor(ctx, localChannelID, spec.Number)
		if err != nil {
			return "", spec.Number, false, err
		}
		if moved != spec.Number {
			e.log.Warn("channel number already used in Tunarr; moving this channel rather than failing",
				"channel", spec.Name, "wanted", spec.Number, "using", moved)
			if e.acts != nil {
				e.acts.Warn(ctx, activitystore.ActivityKindChannel, spec.Name, fmt.Sprintf(
					"%q moved to channel %d — Tunarr already had a channel on %d. Loomarr never changes a channel it didn't create.",
					spec.Name, moved, spec.Number))
			}
			spec.Number = moved
		}
	}
	id, err := e.prog.EnsureChannel(ctx, spec)
	if err != nil {
		return "", spec.Number, false, fmt.Errorf("ensure channel: %w", err)
	}
	return id, spec.Number, created, nil
}

// freeNumberFor returns `want` when neither Tunarr nor another local channel holds it, else the
// lowest number free in both stores. Best-effort for Tunarr: if its list can't be read the caller
// keeps its number and the create either succeeds or fails as before. The local list is required;
// creating remotely on a number the database already owns would manufacture an orphan before the
// unique index rejects the attachment.
func (e *Engine) freeNumberFor(ctx context.Context, localChannelID string, want int) (int, error) {
	existing, err := e.prog.ListChannels(ctx)
	if err != nil {
		e.log.Warn("couldn't read Tunarr's channels to check the number; keeping it", "number", want, "err", err)
		return want, nil //nolint:nilerr // deliberate: a list failure must not renumber a channel
	}
	used := make(map[int]bool, len(existing))
	for _, c := range existing {
		used[c.Number] = true
	}
	local, err := e.store.ListChannels(ctx)
	if err != nil {
		return want, fmt.Errorf("list local channels for number allocation: %w", err)
	}
	for _, c := range local {
		if c.ID != localChannelID {
			used[c.Number] = true
		}
	}
	if !used[want] {
		return want, nil
	}
	for n := 1; ; n++ {
		if !used[n] {
			return n, nil
		}
	}
}

// statusFor derives the channel's Loomarr-side status from its desired lineup.
//
// ⚠ AN EMPTY DECK IS NOT `live`. A channel that computed to nothing must not report healthy:
// the operator's only symptom would be an empty grid, with no hint that filtering was the
// cause (a seasonal bench hid exactly that way: six titles on the lineup, every one benched
// `out_of_season`, `desired_json` literally `[]`).
//
// Checked AFTER drift, because drift is the more specific claim: a channel that both
// drifted and emptied is better described by what changed under it.
//
// This does not collide with `building`, which is set at create time and by edits
// (channels/edit) and never by this function — reconcile only ever runs on a channel
// that has already been created, so "no slots yet" here means filtering removed them,
// not that the channel is still on its way.
func (e *Engine) statusFor(d schedule.DesiredLineup, drifted bool) schedule.ChannelStatus {
	if drifted {
		return schedule.StatusDrifted
	}
	// ⚠ Counts PROGRAMMES, not slots. `len(d.Slots) == 0` would be the wrong predicate:
	// a deck with nothing to play still carries SlotFiller entries (breaks/flex), so it is
	// non-empty while airing no content.
	for _, s := range d.Slots {
		if s.Kind == schedule.SlotProgram {
			return schedule.StatusLive
		}
	}
	return schedule.StatusEmpty
}

// pokeGuide triggers a guide refresh, logging (never returning) failures (§9).
func (e *Engine) pokeGuide(ctx context.Context, channelID string) {
	if e.guide == nil {
		return
	}
	if err := e.guide.PokeGuideRefresh(ctx); err != nil {
		e.log.Warn("guide refresh poke failed (freshness degraded, reconcile ok)",
			"channel", channelID, "err", err)
	}
}

// rescanTuner asks the media server to re-read the tuner channel list so a newly
// created channel is discovered (§9), logging (never returning) failures.
func (e *Engine) rescanTuner(ctx context.Context, channelID string) {
	if e.guide == nil {
		return
	}
	if err := e.guide.RescanTuner(ctx); err != nil {
		e.log.Warn("tuner re-scan failed (new channel may not appear until periodic scan, reconcile ok)",
			"channel", channelID, "err", err)
	}
}
