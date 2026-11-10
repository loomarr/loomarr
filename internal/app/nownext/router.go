// Package nownext answers the channel list's "on now / up next" card and the "what's on later"
// strip, routing each channel to the backend that actually streams it (§9.1).
package nownext

import (
	"context"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
)

// Entry is one programme on a channel card or in the "what's on later" strip. The times are the
// selected playout backend's own schedule.
type Entry struct {
	Title   string
	StartMs int64
	StopMs  int64
	// Gap marks a flex/commercial-pod gap rather than a programme.
	Gap bool
	// TMDBID joins to a provisioning key (movie:tmdb:<id>) when known.
	TMDBID string
}

// ChannelNowNext is what a channel card shows: what is on, and what follows.
type ChannelNowNext struct {
	Now  *Entry
	Next *Entry
}

// Config is what a Router reads.
type Config struct {
	// Tunarr answers for Tunarr-backed channels.
	Tunarr TunarrGuide
	// Internal resolves an internal-playout channel's timeline.
	Internal BroadcastReader
	// Channels reads the channel records the backend decision is made from.
	Channels ChannelLister
	// InternalFor reports whether a channel is served by internal playout. Used only when
	// AppliedBackend is nil.
	InternalFor func(channelstore.Channel) bool
	// AppliedBackend reads the durable backend checkpoint once per router operation.
	AppliedBackend func(context.Context) (string, error)
	// Window is how far ahead to look for the NEXT programme.
	Window time.Duration
}

// New returns the router over c.
func New(c Config) Router {
	return Router{
		tunarr: c.Tunarr, internal: c.Internal, channels: c.Channels,
		internalFor: c.InternalFor, appliedBackend: c.AppliedBackend, window: c.Window,
	}
}

// Router answers "what is on now" from the backend that is actually STREAMING each channel
// (§9.1).
//
// A channel's own backend is the only authority on its schedule: Tunarr and Loomarr's playout
// each run an independent epoch, so a card read from the other backend would contradict the
// guide grid and the XMLTV the television reads. The router therefore dispatches per channel:
//
//   - an internal channel resolves through BroadcastsBetween — the SAME resolver the encoder and
//     the XMLTV guide share, so §9.1's one-source guarantee covers the card too;
//   - a Tunarr channel reads Tunarr's guide;
//   - mixed installs are correct by construction, because `playout.backend` is per-channel
//     overridable (§15). Media-server mixed-tuner wiring is a separate limitation recorded in
//     design §9.1.
//
// The router exposes only Loomarr ids to the API and translates to Tunarr ids internally. Which
// backend serves a channel is decided by schedule.PlaysInternally, the one rule the channel
// builder shares.
type Router struct {
	// tunarr answers for Tunarr-backed channels. Nil ⇒ those channels have no entry.
	tunarr TunarrGuide
	// internal resolves an internal-playout channel's timeline. Nil ⇒ internal channels have
	// no entry; they never fall back to Tunarr's guide, which is a different schedule.
	internal BroadcastReader
	// channels maps a Loomarr channel id to its record, for the backend decision.
	channels ChannelLister
	// internalFor reports whether a channel is served by internal playout.
	internalFor func(channelstore.Channel) bool
	// appliedBackend reads the durable checkpoint once per router operation. Production uses this
	// seam so a request on any Postgres replica routes from the published backend.
	appliedBackend func(context.Context) (string, error)
	// window is how far ahead to look for the NEXT programme. Long enough to contain one, and
	// no longer: this is a read on a list view, not a guide.
	window time.Duration
}

// TunarrGuide is keyed by Tunarr's remote channel ids. Router translates that
// adapter-specific shape into api.GuideReader's Loomarr-id contract at this seam.
type TunarrGuide interface {
	NowNext(ctx context.Context, now time.Time) (map[string]ChannelNowNext, error)
	Upcoming(ctx context.Context, tunarrID string, now time.Time, limit int) ([]Entry, error)
}

// BroadcastReader is the resolver slice the router needs — satisfied by *playoutresolve.Resolver.
type BroadcastReader interface {
	BroadcastsBetween(ctx context.Context, channelID string, from, to time.Time) ([]playout.Broadcast, error)
}

// ChannelLister is the one store read the router makes.
type ChannelLister interface {
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	GetChannel(ctx context.Context, id string) (channelstore.Channel, error)
}

// NowNext returns what is on and what follows, per channel, from the right backend.
//
// The returned map is keyed by Loomarr channel id. Translation from Tunarr's remote ids happens
// here, while an internal channel needs no remote identity at all.
func (r Router) NowNext(ctx context.Context, now time.Time) (map[string]ChannelNowNext, error) {
	channels, err := r.channels.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	applied, err := r.resolveApplied(ctx)
	if err != nil {
		return nil, err
	}

	// Tunarr's guide is ONE upstream call for every channel, so it is read once up front rather
	// than per channel. A failure is not fatal: internal channels can still be answered, and the
	// Tunarr-backed ones degrade to no entry.
	var fromTunarr map[string]ChannelNowNext
	if r.tunarr != nil && r.anyOnTunarr(channels, applied) {
		if g, gerr := r.tunarr.NowNext(ctx, now); gerr == nil {
			fromTunarr = g
		}
	}

	out := make(map[string]ChannelNowNext, len(channels))
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
func (r Router) anyOnTunarr(channels []channelstore.Channel, applied string) bool {
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
// containment test is written out rather than assuming ordering: an overlapping or
// out-of-order timeline must not mislabel what is airing.
func (r Router) internalNowNext(
	ctx context.Context, ch channelstore.Channel, now time.Time,
) (ChannelNowNext, bool) {
	if r.internal == nil {
		return ChannelNowNext{}, false
	}
	bs, err := r.internal.BroadcastsBetween(ctx, ch.ID, now, now.Add(r.window))
	if err != nil || len(bs) == 0 {
		// A channel with nothing scheduled is a real state, not a failure — the card shows
		// "nothing scheduled" rather than blanking the page.
		return ChannelNowNext{}, false
	}

	var current, upcoming *Entry
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
		return ChannelNowNext{}, false
	}
	return ChannelNowNext{Now: current, Next: upcoming}, true
}

// Upcoming returns one channel's "what's on later" strip from the right backend.
func (r Router) Upcoming(
	ctx context.Context, channelID string, now time.Time, limit int,
) ([]Entry, error) {
	if limit <= 0 {
		limit = 6
	}
	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		return []Entry{}, nil
	}
	if !ch.Status.Reconcilable() || ch.Status == schedule.StatusEmpty {
		return []Entry{}, nil
	}
	applied, err := r.resolveApplied(ctx)
	if err != nil {
		return nil, err
	}
	if r.isInternal(ch, applied) {
		return r.internalUpcoming(ctx, ch, now, limit)
	}
	if r.tunarr == nil || ch.TunarrID == "" {
		return []Entry{}, nil
	}
	return r.tunarr.Upcoming(ctx, ch.TunarrID, now, limit)
}

func (r Router) resolveApplied(ctx context.Context) (string, error) {
	if r.appliedBackend != nil {
		return r.appliedBackend(ctx)
	}
	return "", nil
}

func (r Router) isInternal(ch channelstore.Channel, applied string) bool {
	if r.appliedBackend != nil {
		return schedule.PlaysInternally(ch.Policy, applied)
	}
	if r.internalFor != nil {
		return r.internalFor(ch)
	}
	return false
}

func (r Router) internalUpcoming(
	ctx context.Context, ch channelstore.Channel, now time.Time, limit int,
) ([]Entry, error) {
	// A wider window than now/next: the strip lists several programmes, and gaps are dropped
	// below so the span has to cover more than `limit` blocks' worth of wall clock.
	bs, err := r.internal.BroadcastsBetween(ctx, ch.ID, now, now.Add(upcomingWindow))
	if err != nil {
		return []Entry{}, nil // a strip that cannot load shows nothing, never an error
	}
	out := make([]Entry, 0, limit)
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
func broadcastToNowNext(b playout.Broadcast) Entry {
	title := b.Title
	if b.SeriesTitle != "" {
		title = b.SeriesTitle
	}
	return Entry{
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
