package playoutresolve

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/playout"
)

// BroadcastsBetween resolves a channel's programme timeline for the XMLTV guide (§9.1, V6b).
//
// Deliberately on the SAME type as AiringNow. The rolling window on air reads the SAME cycle as
// the encoder (airingCycle); only other windows use CyclePreview as a forecast.
//
// `at` is the window's START, not `now`. CyclePreview evaluates curation rules at an instant —
// a rule that switches the channel to horror at 21:00 changes the lineup — and a guide built at
// `now` would advertise the current rule's lineup for the whole window. Using `from` means the
// listings reflect what the rules said when the window opened; a window spanning a rule
// boundary is a known limitation rather than a silent wrong answer (the mid-window portion
// shows the earlier rule's programmes).
//
// cycleAt is CyclePreview with the arranged cycle memoised (cyclecache.go). It is always a
// FORECAST: the window on air comes from airingCycle, which segmentedBroadcasts reads first.
//
// GUIDE PATHS ONLY. AiringNow deliberately does not use this: it is what ffmpeg streams, and
// §9.1's one-source rule is worth more on the broadcast path than the milliseconds a cache would
// save there (one channel, once per programme, versus every channel on every grid poll).
//
// Degrades to the live computation whenever the cache cannot be trusted: no store, no cache, or
// a channel that will not load or fingerprint. Every one of those returns the same answer
// CyclePreview would — only slower — so a cache failure is a performance regression, never a
// wrong lineup.
// Returns the arranged slots AND the resolved rolling-window horizon, which segmentedBroadcasts
// needs to know where the deck rotates. The window is part of the same CyclePreview answer, so
// carrying it costs nothing and saves the caller re-deriving the rule > channel > default
// precedence (and getting it subtly different).
func (r *Resolver) cycleAt(
	ctx context.Context, channelID string, at time.Time,
) ([]schedule.Slot, time.Duration, error) {
	if r.cycles == nil || r.channels == nil {
		_, slots, _, window, err := r.engine.CyclePreview(ctx, channelID, at)
		return slots, window, err
	}

	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		// The engine loads the channel itself and will surface the real error (or succeed, if
		// this was a transient read) — so fall through rather than failing the row here.
		_, slots, _, window, cerr := r.engine.CyclePreview(ctx, channelID, at)
		return slots, window, cerr
	}

	// The key carries what `at` changes about the arrangement (active rule, active holidays; the
	// rolling-window index is added inside the cache), not the wall-clock — so a navigated window
	// or a later visit reuses the work. See the cycle cache's header.
	_, zone := r.engine.RollingWindow(ch.Policy, at)
	key, ok := fingerprintChannel(channelID, ch.Lineup, ch.Policy, schedule.ClockSignature(ch.Policy, at, r.engine.Calendar()))
	if !ok {
		_, slots, _, window, cerr := r.engine.CyclePreview(ctx, channelID, at)
		return slots, window, cerr
	}

	if slots, window, hit := r.cycles.get(key, at, zone); hit {
		return slots, window, nil
	}

	_, slots, _, window, err := r.engine.CyclePreview(ctx, channelID, at)
	if err != nil {
		return nil, 0, err
	}
	r.cycles.put(key, at, zone, slots, window)
	return slots, window, nil
}

// maxGuideSegments bounds how many rolling windows one guide request will re-resolve.
//
// The FE offers a 7-day forward span (RETENTION_DAYS + FORWARD_DAYS) against a 24h default
// window, so 8 covers the whole picker with a segment to spare. The cap is a backstop against a
// pathological window (a channel configured to a one-minute horizon must not turn one guide
// request into thousands of arrangements), not an expected limit: past it the tail of the span
// keeps the last segment's cycle.
const maxGuideSegments = 8

// segmentedBroadcasts walks [from, to) one ROLLING WINDOW at a time, re-resolving the channel's
// cycle at each turn, and concatenates the results. The window on air is the encoder's own
// (airingCycle). With carry-over (#1675) a window turns where the programme crossing its boundary
// ends, not on the boundary itself (carryOverLegs).
//
// # Why this exists
//
// The scheduler ROTATES: windowSliceDealt advances its start by the window index, so day 0 airs one
// slice of the deck and day 1 continues where it left off (§6.5, "over a full cycle every program
// airs"). Reconciliation commits that rotation into Desired at the new window; AiringNow reads the
// accepted value. A multi-day guide still has to forecast the later windows: resolving ONE cycle
// at `from` and looping it across the whole requested span would advertise a rotation that will
// never happen.
//
// Measured on a 27-title channel at the same instant 24h out: the guide advertised RoboCop / The
// Running Man / The Empire Strikes Back while the scheduler would arrange Lethal Weapon / Lethal
// Weapon 2 / Akira. Not merely under-reporting variety — a wrong EPG.
//
// (The XMLTV document escaped this: it publishes a 24h window, which is one rotation step, so it
// was right by construction. Loomarr's own grid queries up to 7 days, which is where it showed.)
//
// # Why here and not inside playout.BroadcastsBetween
//
// That walk is SHARED with the encoder and is deliberately a pure function of one slot list — it
// is what guarantees the grid and the encoder cannot disagree about what is on at 21:00. Teaching
// it to re-resolve would give it a dependency on the engine and put the invariant at risk. The
// segmentation belongs to the caller that spans multiple windows; each segment still goes through
// the identical shared walk.
//
// A window of 0 (WindowFull / unbounded) means the deck never rotates, so one segment is the
// whole answer.
func (r *Resolver) segmentedBroadcasts(
	ctx context.Context, channelID string, from, to time.Time,
	project func(slots []schedule.Slot, epoch, segFrom, segTo time.Time) []playout.Broadcast,
) ([]playout.Broadcast, error) {
	if r.channels == nil {
		return nil, errors.New("read channel playout anchor: channel reader is not configured")
	}
	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("read channel playout anchor %s: %w", channelID, err)
	}
	// Reconcile stamps the anchor when the channel's first playable deck goes live, so a channel
	// that has never been live has no timeline yet: an empty row, not corrupt state (#1630, which
	// logged a failure for it on every guide read). Live and drifted keep the refusal below.
	if ch.PlayoutAnchor.IsZero() && ch.Status != schedule.StatusLive && ch.Status != schedule.StatusDrifted {
		return nil, nil
	}
	live, err := r.airingCycle(ctx, ch, r.now())
	if err != nil {
		return nil, err
	}
	if live.slots == nil { // nothing accepted yet: forecast the window on air, from its opening
		now := r.now()
		if live.slots, _, err = r.cycleAt(ctx, channelID, now); err != nil {
			return nil, err
		}
		if live.carriesOver && live.window > 0 {
			live.epoch = later(live.epoch, schedule.WindowStart(now, live.window, live.zone))
		}
	}
	// Unbounded window ⇒ no rotation ⇒ nothing to segment.
	if live.window <= 0 {
		return project(live.slots, live.epoch, from, to), nil
	}

	var legs []guideLeg
	if live.carriesOver {
		legs = r.carryOverLegs(ctx, channelID, live, from, to)
	} else {
		legs = r.fixedAnchorLegs(ctx, channelID, live, from, to)
	}
	out := make([]playout.Broadcast, 0, 32)
	for _, leg := range legs {
		segFrom, segTo := later(from, leg.from), earlier(to, leg.to)
		if !segTo.After(segFrom) {
			continue
		}
		// Project keeps the true start/stop of a programme crossing the requested viewport.
		// That is correct at the OUTER Guide edges, but an INTERNAL leg edge changes which
		// arranged cycle is authoritative. Clip there or the outgoing cycle's tail and the
		// incoming cycle's head occupy the same wall-clock time. A carry-over edge is a programme
		// boundary, so the clip is a no-op there; it cuts only where the channel really cuts.
		clipFrom, clipTo := time.Time{}, time.Time{}
		if leg.from.After(from) {
			clipFrom = leg.from
		}
		if leg.to.Before(to) {
			clipTo = leg.to
		}
		for _, broadcast := range project(leg.slots, leg.epoch, segFrom, segTo) {
			if clipped, ok := broadcast.ClipTo(clipFrom, clipTo); ok {
				out = append(out, clipped)
			}
		}
	}
	return out, nil
}

// guideLeg is one stretch of a channel's timeline: `slots` walked from `epoch`, airing over
// [from, to).
type guideLeg struct {
	slots    []schedule.Slot
	epoch    time.Time
	from, to time.Time
}

// carryOverLegs lays out the timeline of a channel whose rolling window turns with carry-over
// (#1675): the window on air runs until the programme crossing its boundary ends, and each later
// window is forecast from where the one before it ends, through the same turn rule reconcile
// commits (playout.CarryOverEnd). No programme is cut at a boundary and none is joined part-way.
//
// Windows BEFORE the one on air are no longer in state (each turn moved the anchor), so each is
// walked from its own opening, the closest estimate of where it began, and the last runs to where
// the window on air began. The past is a reconstruction either way: it is recomputed from the
// current lineup, which is why the guide's lookback is bounded.
//
// One segment failing must not empty the guide — keep what we have. The grid renders a shorter
// forecast rather than nothing, the same posture the per-channel failure takes in
// api.channelGuide.
func (r *Resolver) carryOverLegs(ctx context.Context, channelID string, live airingCycle, from, to time.Time) []guideLeg {
	var legs []guideLeg
	liveOpened := schedule.WindowStart(live.epoch, live.window, live.zone)
	// A `from` between the live window's opening and its anchor still needs the window before:
	// that stretch is its carried-over programme.
	first := schedule.WindowStart(earlier(from, liveOpened.Add(-time.Nanosecond)), live.window, live.zone)
	for opened := first; from.Before(live.epoch) && opened.Before(liveOpened) && opened.Before(to) && len(legs) < maxGuideSegments; {
		next := schedule.NextWindowStart(opened, live.window, live.zone)
		slots, _, err := r.cycleAt(ctx, channelID, opened)
		if err != nil {
			break
		}
		leg := guideLeg{slots: slots, epoch: opened, from: opened, to: next}
		if !next.Before(liveOpened) {
			leg.to = live.epoch
		}
		legs = append(legs, leg)
		opened = next
	}

	leg := guideLeg{slots: live.slots, epoch: live.epoch, from: live.epoch}
	for {
		leg.to = playout.CarryOverEnd(leg.slots, leg.epoch, schedule.NextWindowStart(leg.epoch, live.window, live.zone))
		legs = append(legs, leg)
		if !leg.to.Before(to) || len(legs) >= maxGuideSegments {
			break
		}
		slots, _, err := r.cycleAt(ctx, channelID, leg.to)
		if err != nil {
			break
		}
		leg = guideLeg{slots: slots, epoch: leg.to, from: leg.to}
	}
	return legs
}

// fixedAnchorLegs lays out the timeline of a channel that cannot carry over (Tunarr-backed; see
// CarriesOver): every rolling window is walked from the channel's one fixed anchor and cut at the
// window's edges, because that is where the backend swaps the list.
func (r *Resolver) fixedAnchorLegs(ctx context.Context, channelID string, live airingCycle, from, to time.Time) []guideLeg {
	var legs []guideLeg
	onAir := schedule.WindowStart(r.now(), live.window, live.zone)
	for segFrom := from; segFrom.Before(to) && len(legs) < maxGuideSegments; {
		next := schedule.NextWindowStart(segFrom, live.window, live.zone)
		slots := live.slots
		if !schedule.WindowStart(segFrom, live.window, live.zone).Equal(onAir) {
			var err error
			if slots, _, err = r.cycleAt(ctx, channelID, segFrom); err != nil {
				break
			}
		}
		legs = append(legs, guideLeg{slots: slots, epoch: live.epoch, from: segFrom, to: next})
		segFrom = next
	}
	return legs
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (r *Resolver) BroadcastsBetween(
	ctx context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	bs, err := r.segmentedBroadcasts(ctx, channelID, from, to, playout.BroadcastsBetween)
	if err != nil {
		return nil, err
	}
	r.attachMetadata(ctx, bs)
	return bs, nil
}

// BroadcastsWithPending is BroadcastsBetween plus pending acquisitions, for the time grid
// (V13b). Same CyclePreview, same epoch, same arithmetic — only the projection differs, so the
// grid cannot disagree with the encoder about what airs when.
func (r *Resolver) BroadcastsWithPending(
	ctx context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	bs, err := r.segmentedBroadcasts(ctx, channelID, from, to, playout.BroadcastsWithPending)
	if err != nil {
		return nil, err
	}
	r.attachMetadata(ctx, bs)
	r.attachProvenance(ctx, bs)
	return bs, nil
}

// attachProvenance fills in each block's one-line "why is this here" (§12 hover card).
//
// Only for the GRID, never the XMLTV guide: an EPG lists what is on, and "acquiring · 62%" is
// an operator's answer, not a viewer's listing. That split is why this is a separate pass
// rather than part of attachMetadata.
//
// BEST-EFFORT, like the metadata pass: a store hiccup leaves the blocks exactly as they were.
// A hover card missing one line is far better than a guide that fails to load.
func (r *Resolver) attachProvenance(ctx context.Context, bs []playout.Broadcast) {
	if r.titles == nil || len(bs) == 0 {
		return
	}
	now := r.now()
	// One lookup per DISTINCT key: a channel airing six episodes of one series shares a
	// single acquisition record, so keying the cache on the provisioning key collapses those
	// to one read.
	cache := map[provision.Key]string{}
	for i := range bs {
		k := bs[i].Key
		if k == "" {
			continue // filler/flex have no acquisition to describe
		}
		if p, ok := cache[k]; ok {
			bs[i].Provenance = p
			continue
		}
		rec, err := r.titles.GetTitle(ctx, k)
		if err != nil {
			cache[k] = "" // remember the miss too, so one bad key is not re-read per block
			continue
		}
		p := provenanceOf(rec, now)
		cache[k] = p
		bs[i].Provenance = p
	}
}

// attachMetadata fills in descriptions, genres, years and ratings from the Library's tables, in
// ONE read for the whole channel (#2151 step 16).
//
// BEST-EFFORT BY DESIGN. A failure leaves the broadcasts exactly as they were — titles and times
// intact — because a guide with thin entries is far better than no guide.
func (r *Resolver) attachMetadata(ctx context.Context, bs []playout.Broadcast) {
	if r.itemFacts == nil || len(bs) == 0 {
		return
	}
	ids := make([]string, 0, len(bs))
	for _, b := range bs {
		if b.LibraryItemID != "" {
			ids = append(ids, b.LibraryItemID)
		}
	}
	if len(ids) == 0 {
		return
	}

	meta, err := ItemMetadata{Facts: r.itemFacts}.ItemMetadataByID(ctx, ids)
	if err != nil && r.log != nil {
		// Logged, not returned: the partial map is still applied below, so a failing read does
		// not cost the whole document its descriptions.
		r.log.Debug("playout: guide metadata partially unavailable",
			"err", err, "resolved", len(meta), "wanted", len(ids))
	}
	for i := range bs {
		m, ok := meta[bs[i].LibraryItemID]
		if !ok {
			continue // removed from the library since the lineup was built; keep the title
		}
		bs[i].Description = m.Overview
		bs[i].Genres = m.Genres
		bs[i].Year = m.Year
		bs[i].Rating = m.OfficialRating
		bs[i].RuntimeMs = m.RuntimeMs
	}
}
