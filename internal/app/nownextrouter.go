package app

import (
	"context"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// nowNextRouter answers "what is on now" from the backend that is actually STREAMING each
// channel (§9.1).
//
// # The bug this exists for
//
// `GET /v1/channels/now-next` and `…/{id}/upcoming` once read TUNARR's guide by TunarrID, and
// were wired on `tunarr.url != ""` alone — with no reference to `playout.backend`. A channel
// reconciled to Tunarr in the past keeps its `tunarr_id`, and Tunarr keeps generating listings
// for it, so after a switch to internal playout these endpoints kept answering confidently from
// a schedule with its own independent epoch.
//
// Observed on the dev install, same instant, one channel:
//
//	/v1/guide + /playout/guide.xml   The Last Jedi        (start 1785165161848)
//	/v1/channels/now-next            The Rise of Skywalker (start 1785167001088)
//
// ~30 minutes apart. Neither was stale in the caching sense — they were two different
// schedules, and the card contradicted both the grid and the XMLTV the television reads.
//
// # Why a router rather than a fix to either reader
//
// Both readers are correct FOR THE CHANNELS THEIR BACKEND STREAMS. The defect was that one of
// them answered for every channel unconditionally. Dispatching per channel means:
//
//   - an internal channel resolves through BroadcastsBetween — the SAME resolver the encoder and
//     the XMLTV guide already share, so §9.1's one-source guarantee now covers the card too;
//   - a Tunarr channel keeps reading Tunarr's guide, unchanged;
//   - Loomarr's own mixed-backend reads are correct by construction, which matters because
//     `playout.backend` is per-channel overridable (§15). Media-server mixed-tuner wiring is a
//     separate limitation recorded in design §9.1.
//
// The router exposes only Loomarr ids to the API and translates to Tunarr ids internally. The
// precedence is the same nil-means-inherit shape playoutChannels uses. Duplicating that rule
// here would be a second place to get it wrong, so it is resolved by the shared playsInternally
// helper below.
type nowNextRouter struct {
	// tunarr answers for Tunarr-backed channels. Nil ⇒ those channels simply have no entry,
	// which is the pre-existing "no guide reader configured" behaviour.
	tunarr tunarrGuideReader
	// internal resolves an internal-playout channel's timeline. Nil ⇒ internal channels have
	// no entry rather than falling back to Tunarr's guide — falling back is exactly the bug.
	internal broadcastReader
	// channels maps a Loomarr channel id to its record, for the backend decision.
	channels channelLister
	// internalFor reports whether a channel is served by internal playout.
	internalFor func(store.Channel) bool
	// appliedBackend reads the durable checkpoint once per router operation. Production uses this
	// seam so a request on any Postgres replica routes from the published backend.
	appliedBackend func(context.Context) (string, error)
	// window is how far ahead to look for the NEXT programme. Long enough to contain one, and
	// no longer: this is a read on a list view, not a guide.
	window time.Duration
}

// tunarrGuideReader is keyed by Tunarr's remote channel ids. nowNextRouter translates that
// adapter-specific shape into api.GuideReader's Loomarr-id contract at this seam.
type tunarrGuideReader interface {
	NowNext(ctx context.Context, now time.Time) (map[string]api.ChannelNowNext, error)
	Upcoming(ctx context.Context, tunarrID string, now time.Time, limit int) ([]api.NowNextEntry, error)
}

// broadcastReader is the resolver slice the router needs — satisfied by *playoutResolver.
type broadcastReader interface {
	BroadcastsBetween(ctx context.Context, channelID string, from, to time.Time) ([]playout.Broadcast, error)
}

// channelLister is the one store read the router makes.
type channelLister interface {
	ListChannels(ctx context.Context) ([]store.Channel, error)
	GetChannel(ctx context.Context, id string) (store.Channel, error)
}

// NowNext returns what is on and what follows, per channel, from the right backend.
//
// The returned map is keyed by Loomarr channel id. Translation from Tunarr's remote ids happens
// here, while an internal channel needs no remote identity at all.
func (r nowNextRouter) NowNext(ctx context.Context, now time.Time) (map[string]api.ChannelNowNext, error) {
	channels, err := r.channels.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	applied, err := r.resolveApplied(ctx)
	if err != nil {
		return nil, err
	}

	// Tunarr's guide is ONE upstream call for every channel, so it is read once up front rather
	// than per channel — the property the original adapter was built around, preserved. A
	// failure is not fatal: internal channels can still be answered, and the Tunarr-backed ones
	// degrade to no entry exactly as they did when the guide was unreachable.
	var fromTunarr map[string]api.ChannelNowNext
	if r.tunarr != nil && r.anyOnTunarr(channels, applied) {
		if g, gerr := r.tunarr.NowNext(ctx, now); gerr == nil {
			fromTunarr = g
		}
	}

	out := make(map[string]api.ChannelNowNext, len(channels))
	for _, ch := range channels {
		if !ch.Status.Reconcilable() || ch.Status == schedule.StatusEmpty {
			continue // paused/detached channels are deliberately off Loomarr's guide surfaces
		}
		if !r.isInternal(ch, applied) {
			if ch.TunarrID == "" {
				continue // Tunarr-backed but not yet created there: nothing can be airing.
			}
			if nn, ok := fromTunarr[ch.TunarrID]; ok {
				out[ch.ID] = nn
			}
			continue
		}
		if nn, ok := r.internalNowNext(ctx, ch, now); ok {
			out[ch.ID] = nn
		}
	}
	return out, nil
}

// anyOnTunarr reports whether the Tunarr guide is worth reading at all.
//
// An install that has moved every channel to internal playout should not pay a round trip to
// Tunarr on every list render — and, more importantly, should not have its list view degraded by
// a Tunarr that is slow or down when nothing on screen depends on it.
func (r nowNextRouter) anyOnTunarr(channels []store.Channel, applied string) bool {
	for _, ch := range channels {
		if ch.Status.Reconcilable() && ch.Status != schedule.StatusEmpty &&
			ch.TunarrID != "" && !r.isInternal(ch, applied) {
			return true
		}
	}
	return false
}

// internalNowNext reduces an internal channel's timeline to the now/next pair.
//
// The window is asked for from `now`, so the FIRST broadcast containing `now` is what is on. The
// containment test is written out rather than assuming ordering, matching guideAdapter — an
// overlapping or out-of-order timeline must not mislabel what is airing.
func (r nowNextRouter) internalNowNext(
	ctx context.Context, ch store.Channel, now time.Time,
) (api.ChannelNowNext, bool) {
	if r.internal == nil {
		return api.ChannelNowNext{}, false
	}
	bs, err := r.internal.BroadcastsBetween(ctx, ch.ID, now, now.Add(r.window))
	if err != nil || len(bs) == 0 {
		// A channel with nothing scheduled is a real state, not a failure — the card shows
		// "nothing scheduled" rather than blanking the page.
		return api.ChannelNowNext{}, false
	}

	var current, upcoming *api.NowNextEntry
	for _, b := range bs {
		entry := broadcastToNowNext(b)
		switch {
		case !b.Start.After(now) && now.Before(b.Stop) && current == nil:
			e := entry
			current = &e
		case b.Start.After(now) && (upcoming == nil || entry.StartMs < upcoming.StartMs):
			e := entry
			upcoming = &e
		}
	}
	if current == nil && upcoming == nil {
		return api.ChannelNowNext{}, false
	}
	return api.ChannelNowNext{Now: current, Next: upcoming}, true
}

// Upcoming returns one channel's "what's on later" strip from the right backend.
func (r nowNextRouter) Upcoming(
	ctx context.Context, channelID string, now time.Time, limit int,
) ([]api.NowNextEntry, error) {
	if limit <= 0 {
		limit = 6
	}
	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		return []api.NowNextEntry{}, nil
	}
	if !ch.Status.Reconcilable() || ch.Status == schedule.StatusEmpty {
		return []api.NowNextEntry{}, nil
	}
	applied, err := r.resolveApplied(ctx)
	if err != nil {
		return nil, err
	}
	if r.isInternal(ch, applied) {
		return r.internalUpcoming(ctx, ch, now, limit)
	}
	if r.tunarr == nil || ch.TunarrID == "" {
		return []api.NowNextEntry{}, nil
	}
	return r.tunarr.Upcoming(ctx, ch.TunarrID, now, limit)
}

func (r nowNextRouter) resolveApplied(ctx context.Context) (string, error) {
	if r.appliedBackend != nil {
		return r.appliedBackend(ctx)
	}
	return "", nil
}

func (r nowNextRouter) isInternal(ch store.Channel, applied string) bool {
	if r.appliedBackend != nil {
		return schedule.PlaysInternally(ch.Policy, applied)
	}
	if r.internalFor != nil {
		return r.internalFor(ch)
	}
	return false
}

func (r nowNextRouter) internalUpcoming(
	ctx context.Context, ch store.Channel, now time.Time, limit int,
) ([]api.NowNextEntry, error) {
	// A wider window than now/next: the strip lists several programmes, and gaps are dropped
	// below so the span has to cover more than `limit` blocks' worth of wall clock.
	bs, err := r.internal.BroadcastsBetween(ctx, ch.ID, now, now.Add(upcomingWindow))
	if err != nil {
		return []api.NowNextEntry{}, nil // a strip that cannot load shows nothing, never an error
	}
	out := make([]api.NowNextEntry, 0, limit)
	for _, b := range bs {
		entry := broadcastToNowNext(b)
		if entry.Gap {
			continue // the strip lists shows, not the break padding between them
		}
		if !b.Stop.After(now) {
			continue // already finished
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// upcomingWindow is how far ahead the "what's on later" strip looks.
//
// Twelve hours comfortably holds six programmes of any realistic length, including a channel of
// three-hour films, without asking the resolver to arrange a week of schedule for a strip that
// shows six rows.
const upcomingWindow = 12 * time.Hour

// broadcastToNowNext converts the resolver's block to the card's entry shape.
//
// SeriesTitle is folded into Title for a series so the card reads "The Simpsons" rather than an
// episode name with no context — the same choice the XMLTV writer makes for `<title>`, and the
// one the FE's card layout is built around.
func broadcastToNowNext(b playout.Broadcast) api.NowNextEntry {
	title := b.Title
	if b.SeriesTitle != "" {
		title = b.SeriesTitle
	}
	return api.NowNextEntry{
		Title:   title,
		StartMs: b.Start.UnixMilli(),
		StopMs:  b.Stop.UnixMilli(),
		Gap:     b.Kind != schedule.SlotProgram,
		TMDBID:  tmdbIDFromKey(b.Key),
	}
}

// tmdbIDFromKey lifts the numeric id out of a `movie:tmdb:<id>` provisioning key.
//
// Only the tmdb source, deliberately: NowNextEntry documents this field as joining back to
// `movie:tmdb:<id>`, and a tvdb series key ("series:tvdb:71663") carries a DIFFERENT id space.
// Returning a tvdb id in a field the FE reads as tmdb would look plausible and link to the wrong
// title — worse than the empty string the field is already optional for.
func tmdbIDFromKey(k provision.Key) string {
	parts := strings.Split(string(k), ":")
	if len(parts) != 3 || parts[1] != "tmdb" {
		return ""
	}
	return parts[2]
}
