package channels

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"golang.org/x/sync/errgroup"
)

// "Which upcoming slots does this edit change?" — the Programming tab's change list (#1877).
//
// CyclePreviewDraft answers "what airs at T". This answers the question an editor actually has
// before pressing Apply: across the next hours, where does the draft air something different from
// the saved channel? Both sides are FORECAST with the same walk, from the same instant, against
// the same anchor, availability and engine — so a slot the edit does not touch is computed by
// identical calls on both sides and compares equal. Comparing the draft against the accepted
// (persisted) cycle instead would report every difference between a stale accepted cycle and a
// fresh arrangement as if the edit had caused it.
//
// Read-only, like every preview: nothing persists and no backend is called.

// MaxScheduleDiffHorizon caps how far ahead one diff forecasts. A week is the guide's own span
// (guideMaxWindow) and the longest an editor scrolls; past it a request is a month of
// arrangements for an answer nobody reads.
const MaxScheduleDiffHorizon = 7 * 24 * time.Hour

// defaultScheduleDiffHorizon is the horizon for a channel whose window is unbounded (the whole
// run is one arrangement), where "the schedule window" names no span.
const defaultScheduleDiffHorizon = 24 * time.Hour

// maxForecastSegments bounds how many arrangements one side of a diff computes. A week of daily
// windows with a twice-daily rule is ~21; the cap is a backstop against a pathological policy (an
// hourly rule over a one-hour window), and the span actually compared is reported, never padded.
const maxForecastSegments = 96

// ScheduleDiff is the change list between a channel's saved programming and an unsaved draft.
type ScheduleDiff struct {
	// From and To are the span actually compared. To is earlier than requested only when a side
	// hit maxForecastSegments, so a caller never presents an uncompared tail as unchanged.
	From, To time.Time
	// Compared is how many saved-side programmes the span holds, so "no changes" can be told
	// apart from "nothing is scheduled".
	Compared int
	// Changes are the instants where the two sides air different things, in time order.
	Changes []SlotChange
}

// SlotChange is one instant where the draft airs something different from the saved channel.
type SlotChange struct {
	// Start is where the difference begins; End is where either side's airing ends, the first
	// instant the pairing could change.
	Start, End time.Time
	// Before and After are what each side airs at Start. Nil means nothing airable airs there.
	Before, After *ForecastAiring
}

// ForecastAiring is one block of a forecast timeline.
type ForecastAiring struct {
	Kind        schedule.SlotKind
	Key         provision.Key
	Title       string
	SeriesTitle string
	Season      int
	Episode     int
	// Start and Stop are clipped where the arrangement changes (a rule boundary or a window
	// turn), because that is where the channel switches cycles.
	Start, Stop time.Time
	// Rule is the curation rule whose arrangement this block comes from.
	Rule schedule.ActiveRuleAttribution
}

// sameContent is the diff's identity: what airs, not exactly when it started. A programme that
// is cut at a rule boundary on one side is still the same programme while both sides air it.
func (a *ForecastAiring) sameContent(b *ForecastAiring) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Kind == b.Kind && a.Key == b.Key && a.Title == b.Title && a.Season == b.Season && a.Episode == b.Episode
}

// ScheduleDiffDraft forecasts [from, from+horizon) for the saved channel and for the draft, and
// returns where they differ. Draft semantics are CyclePreviewDraft's (nil = the saved value). A
// zero `from` is the engine clock, read once and shared by both sides; a horizon ≤ 0 is the
// saved channel's rolling window at `from`, capped at MaxScheduleDiffHorizon.
func (e *Engine) ScheduleDiffDraft(
	ctx context.Context, channelID string, from time.Time, horizon time.Duration,
	draftLineup []schedule.LineupEntry, draftPolicy *schedule.ChannelPolicy,
) (ScheduleDiff, error) {
	saved, err := e.store.GetChannel(ctx, channelID)
	if err != nil {
		return ScheduleDiff{}, fmt.Errorf("load channel %s: %w", channelID, err)
	}
	if from.IsZero() {
		from = e.now()
	}
	// Rules match on the instant's own hour and weekday, and reconcile arranges at e.now() — so
	// evaluate in the engine clock's location, not whatever offset a caller's RFC3339 carried.
	from = from.In(e.now().Location())
	if horizon <= 0 {
		horizon, _ = e.RollingWindow(saved.Policy, from)
		if horizon <= 0 {
			horizon = defaultScheduleDiffHorizon
		}
	}
	to := from.Add(min(horizon, MaxScheduleDiffHorizon))

	// The sides are independent CPU-bound walks over read-only inputs, so they run side by side:
	// the request costs one forecast of latency, not two.
	var before, after []ForecastAiring
	var beforeEnd, afterEnd time.Time
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		before, beforeEnd, err = e.forecast(gctx, saved, from, to)
		return err
	})
	g.Go(func() (err error) {
		after, afterEnd, err = e.forecast(gctx, withDraft(saved, draftLineup, draftPolicy), from, to)
		return err
	})
	if err := g.Wait(); err != nil {
		return ScheduleDiff{}, err
	}
	for _, end := range []time.Time{beforeEnd, afterEnd} {
		if end.Before(to) {
			to = end
		}
	}

	compared := 0
	for _, b := range before {
		if b.Kind == schedule.SlotProgram && b.Start.Before(to) {
			compared++
		}
	}
	return ScheduleDiff{From: from, To: to, Compared: compared, Changes: diffForecasts(before, after, from, to)}, nil
}

// withDraft stands the draft in for the saved lineup and policy exactly as CyclePreviewDraft
// does: the lineup lowered like an operator PATCH (Replace + PreserveByKey), the policy whole.
// The anchor, id and history stay the saved channel's — they are not part of an edit.
func withDraft(ch store.Channel, draftLineup []schedule.LineupEntry, draftPolicy *schedule.ChannelPolicy) store.Channel {
	if draftLineup != nil {
		ch.Lineup = schedule.ApplyLineup(ch.Lineup, draftLineup, schedule.LineupReplace, schedule.ApplyOpts{PreserveByKey: true})
	}
	if draftPolicy != nil {
		ch.Policy = *draftPolicy
	}
	return ch
}

// forecast walks one channel's timeline over [from, to), re-arranging wherever the channel
// would: at each rolling-window turn, and at each instant the clock changes the arrangement (a
// rule starting or ending, a holiday). It returns the blocks and the instant the walk reached.
//
// The window turns follow the guide's forecast (playoutResolver.segmentedBroadcasts): with
// carry-over the next window starts where the programme crossing the boundary ends; without it
// (Tunarr) the window is cut on the boundary and walked from the fixed anchor. A rule boundary
// keeps the epoch and cuts, which is what reconcile does when it re-arranges mid-window. One
// deliberate difference: while the accepted window is still on air the guide walks the persisted
// cycle, but here that window is re-arranged under this side's policy (WindowOpened), because a
// save re-arranges it too — reading the persisted cycle would hide the draft's effect on it.
//
// Each arrangement is PreviewPlannedChannel — the exact computation the single-point preview
// runs — memoised on what the clock can change about it (ClockSignature + window index), so a
// week of a daily rule costs one arrangement per distinct (rule, window), not one per boundary.
func (e *Engine) forecast(ctx context.Context, ch store.Channel, from, to time.Time) ([]ForecastAiring, time.Time, error) {
	carries, err := e.CarriesOver(ctx, ch.Policy)
	if err != nil {
		return nil, time.Time{}, err
	}
	type memoKey struct {
		clock  uint64
		window time.Duration
		index  int64
	}
	memo := map[memoKey]CycleResult{}
	// arrange is the cycle the channel airs from `at`. With carry-over it is arranged for the
	// window the epoch opened (WindowOpened), exactly as reconcile arranges the accepted window
	// while its carried-over programme is still on air; without it, for the window `at` is in.
	arrange := func(at, epoch time.Time) (CycleResult, time.Duration, *time.Location, error) {
		window, zone := e.RollingWindow(ch.Policy, at)
		arranged := ch
		arranged.WindowOpened = time.Time{}
		if carries && window > 0 {
			arranged.WindowOpened = schedule.WindowStart(epoch, window, zone)
		}
		indexAt := at
		if !arranged.WindowOpened.IsZero() {
			indexAt = arranged.WindowOpened
		}
		key := memoKey{schedule.ClockSignature(ch.Policy, at), window, schedule.WindowIndex(indexAt, window, zone)}
		if r, ok := memo[key]; ok {
			return r, window, zone, nil
		}
		r, err := e.PreviewPlannedChannel(ctx, arranged, at, e.avail)
		if err != nil {
			return CycleResult{}, 0, nil, err
		}
		memo[key] = r
		return r, window, zone, nil
	}

	// The first epoch is the one the channel airs from at `from`. With carry-over that is
	// playout.WindowTurn over the ACCEPTED cycle — the rule reconcile and the encoder apply: the
	// accepted window stays on air until the programme crossing its boundary ends. A draft does
	// not change history, so both sides take the same accepted cycle and anchor here.
	window, zone := e.RollingWindow(ch.Policy, from)
	epoch := ch.PlayoutAnchor
	switch {
	case window > 0 && carries && !epoch.IsZero():
		_, epoch, _ = playout.WindowTurn(ch.Desired, ch.PlayoutAnchor, window, zone, from)
	case window > 0 && epoch.IsZero():
		// Never live, so never anchored: forecast from the opening of the window on air, which
		// is where reconcile's first anchor would land (later than the guide, which shows nothing).
		epoch = schedule.WindowStart(from, window, zone)
	case epoch.IsZero():
		epoch = from
	}

	var out []ForecastAiring
	cursor := from
	for n := 0; cursor.Before(to); n++ {
		if n == maxForecastSegments {
			return out, cursor, nil
		}
		r, window, zone, err := arrange(cursor, epoch)
		if err != nil {
			return nil, time.Time{}, err
		}

		end, turns := to, false
		if window > 0 {
			turn := schedule.NextWindowStart(cursor, window, zone)
			if carries {
				turn = playout.CarryOverEnd(r.Slots, epoch, schedule.NextWindowStart(epoch, window, zone))
				if !turn.After(cursor) {
					// This arrangement's crossing programme already ended (a rule re-arranged the
					// window): the window turns here.
					epoch = cursor
					continue
				}
			}
			if turn.Before(end) {
				end, turns = turn, true
			}
		}
		if change := nextClockChange(ch.Policy, cursor, end); !change.IsZero() {
			end, turns = change, false
		}

		// Clip only at INTERNAL edges, as the guide does: at `from` a programme in progress keeps
		// its real start; inside the span the arrangement really switches.
		clipFrom, clipTo := time.Time{}, time.Time{}
		if cursor.After(from) {
			clipFrom = cursor
		}
		if end.Before(to) {
			clipTo = end
		}
		for _, b := range playout.BroadcastsBetween(r.Slots, epoch, cursor, end) {
			b, ok := b.ClipTo(clipFrom, clipTo)
			if !ok || (b.Kind != schedule.SlotProgram && b.Kind != schedule.SlotFiller) {
				continue
			}
			out = append(out, ForecastAiring{
				Kind: b.Kind, Key: b.Key, Title: b.Title, SeriesTitle: b.SeriesTitle,
				Season: b.Season, Episode: b.Episode, Start: b.Start, Stop: b.Stop, Rule: r.Active,
			})
		}
		if turns && carries {
			epoch = end
		}
		// Window turns come back in the window zone; rules read the instant's own hour, so keep
		// every probe in the one location the forecast started in.
		cursor = end.In(from.Location())
	}
	return out, to, nil
}

// nextClockChange is the first instant in (after, before) at which the clock changes what the
// arrangement reads (schedule.ClockSignature), or zero when it does not change. Rule hours and
// holidays turn on the hour, so the hour tops are checked, plus each rule's explicit date edges.
func nextClockChange(policy schedule.ChannelPolicy, after, before time.Time) time.Time {
	base := schedule.ClockSignature(policy, after)
	var candidates []time.Time
	top := time.Date(after.Year(), after.Month(), after.Day(), after.Hour(), 0, 0, 0, after.Location()).Add(time.Hour)
	for t := top; t.Before(before); t = t.Add(time.Hour) {
		candidates = append(candidates, t)
	}
	for _, r := range policy.Rules {
		var edges []time.Time
		if r.When.DateFrom != nil {
			edges = append(edges, *r.When.DateFrom)
		}
		if r.When.DateTo != nil {
			edges = append(edges, r.When.DateTo.Add(time.Nanosecond)) // inclusive: stops matching just after
		}
		for _, t := range edges {
			if t.After(after) && t.Before(before) {
				candidates = append(candidates, t)
			}
		}
	}
	slices.SortFunc(candidates, func(a, b time.Time) int { return a.Compare(b) })
	for _, t := range candidates {
		if schedule.ClockSignature(policy, t) != base {
			return t
		}
	}
	return time.Time{}
}

// diffForecasts compares the two timelines at every instant a programme starts on either side
// (clamped to `from`), and reports the instants where they air different content. Break starts
// are not probe points: a break that moves with its programme would otherwise report every
// shifted pod as a change of its own.
func diffForecasts(before, after []ForecastAiring, from, to time.Time) []SlotChange {
	var instants []time.Time
	for _, side := range [][]ForecastAiring{before, after} {
		for _, a := range side {
			if a.Kind == schedule.SlotProgram && a.Start.Before(to) {
				instants = append(instants, later(a.Start, from))
			}
		}
	}
	slices.SortFunc(instants, func(a, b time.Time) int { return a.Compare(b) })
	instants = slices.CompactFunc(instants, func(a, b time.Time) bool { return a.Equal(b) })

	changes := []SlotChange{}
	for _, t := range instants {
		b, a := airingAt(before, t), airingAt(after, t)
		if b.sameContent(a) {
			continue
		}
		end := to
		for _, side := range []*ForecastAiring{b, a} {
			if side != nil && side.Stop.Before(end) {
				end = side.Stop
			}
		}
		changes = append(changes, SlotChange{Start: t, End: end, Before: b, After: a})
	}
	return changes
}

// airingAt is the block covering `t` on a time-ordered timeline, or nil.
func airingAt(timeline []ForecastAiring, t time.Time) *ForecastAiring {
	i := sort.Search(len(timeline), func(i int) bool { return timeline[i].Stop.After(t) })
	if i < len(timeline) && !timeline[i].Start.After(t) {
		a := timeline[i]
		return &a
	}
	return nil
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
