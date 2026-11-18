package pod

import (
	"context"
	"time"
)

// A break is assembled once and stored (#3838 step 2). Reconcile plans a channel's breaks ahead, in
// order, from the household's real airings and the breaks it has already planned, and cuts each
// break slot to the media its pod holds, so the programme resumes when the break's own clips end.
// The guide, the previews and the stream read the stored pod, so what the guide shows is what airs.

// StoredBreak is one assembled break kept for its channel and start.
type StoredBreak struct {
	ChannelID string
	Start     time.Time
	// Ceiling is the slot length the pod was assembled to.
	Ceiling   time.Duration
	Pod       Pod
	PlannedAt time.Time
}

// Length is the media the stored pod holds: the break's real length.
func (b StoredBreak) Length() time.Duration { return time.Duration(b.Pod.MediaMs()) * time.Millisecond }

// MediaMs is the duration of the real files in the pod. The embedded fallback card has no file and
// contributes nothing: it is generated at playout time and must not extend a break.
func (p Pod) MediaMs() int64 {
	var total int64
	for _, e := range p.Entries {
		if e.Path != "" && e.DurationMs > 0 {
			total += e.DurationMs
		}
	}
	return total
}

// BreakStore keeps the stored breaks (implemented by the store).
type BreakStore interface {
	SaveBreaks(ctx context.Context, breaks []StoredBreak) error
	// LoadBreaks returns a channel's stored breaks starting in [from, to].
	LoadBreaks(ctx context.Context, channelID string, from, to time.Time) ([]StoredBreak, error)
	// KeepBreaks deletes a channel's stored breaks starting in [from, until) other than the ones
	// starting at `starts`; a zero bound is open.
	KeepBreaks(ctx context.Context, channelID string, from, until time.Time, starts []time.Time) error
}

// WithBreaks gives the adapter the store that makes breaks assemble once. Without one every break
// is assembled live, as before.
func (a *Adapter) WithBreaks(s BreakStore) *Adapter {
	a.breaks = s
	return a
}

// BreakPlan is one break slot to plan: the non-break time since the previous break (or the start
// of the plan), and the slot length the pod is assembled to.
type BreakPlan struct {
	Gap     time.Duration
	Ceiling time.Duration
}

// PlanBounds places one plan on its channel's timeline.
type PlanBounds struct {
	// From is the first break's start; each later one follows the plans' gaps and the real lengths.
	From time.Time
	// FixedBefore: a break starting before it is on air or aired. It keeps the length it was stored
	// with, or its slot's when it was never stored, and is never assembled, so planning at any
	// instant walks the timeline that aired.
	FixedBefore time.Time
	// Until: a break starting at or after it is not this plan's (it lies past the window's turn,
	// where the next window airs instead). It keeps its slot and nothing is stored for it. Zero: no end.
	Until time.Time
	// ClearFrom: the plan deletes stored breaks in [ClearFrom, Until) that it did not plan. The window
	// on air clears everything behind it (zero); the next window, planned ahead from the turn,
	// clears only from the turn, so neither plan deletes the other's breaks.
	ClearFrom time.Time
}

// evenings of planned airings reach back this far: the longest an evening lasts.
const planLookback = maxEvening

// PlanBreaks assembles and stores the channel's breaks that are not stored yet and returns each
// break's real length (0 when nothing airs in it). The breaks start at bounds.From plus each plan's
// gap and the lengths of the breaks before it, so a break's start depends on the ones planned ahead
// of it. A break already stored at its start keeps its pod, if it was assembled to the same ceiling
// and its clips are still in the catalog; one whose start moved has nothing stored and is assembled
// fresh. See PlanBounds for the fixed breaks, the plan's end, and what it clears.
//
// The history a break is assembled from is the household's real airings up to now plus this
// channel's own breaks already stored before it, so no ad repeats within the channel's evening.
// Other channels' planned breaks do not count: what they air depends on viewing that has not
// happened yet.
//
// It returns nil lengths and no error when the adapter has no break store: the caller keeps the
// slot lengths it has.
func (a *Adapter) PlanBreaks(ctx context.Context, channelID string, sel Selection, seedAt func(startMs int64) int64,
	bounds PlanBounds, plans []BreakPlan) ([]time.Duration, error) {
	if a.breaks == nil || len(plans) == 0 {
		return nil, nil
	}
	from := bounds.From
	clips, err := a.catalog.AllClips(ctx)
	if err != nil {
		return nil, err
	}
	inCatalog := make(map[string]airWindow, len(clips))
	for _, c := range clips {
		inCatalog[c.ID()] = airWindow{startMs: c.AirStartMs, lengthMs: c.DurationMs}
	}
	history := AiringHistory{}
	if a.airings != nil {
		if history, err = a.airings.FillerAirings(ctx); err != nil {
			return nil, err
		}
	}
	horizon := from
	for _, p := range plans {
		horizon = horizon.Add(p.Gap + p.Ceiling)
	}
	stored, err := a.breaks.LoadBreaks(ctx, channelID, from.Add(-planLookback), horizon)
	if err != nil {
		return nil, err
	}
	byStart := make(map[int64]StoredBreak, len(stored))
	var planned []Airing // this channel's breaks before the one being planned, as airings
	for _, b := range stored {
		byStart[b.Start.UnixMilli()] = b
		if b.Start.Before(from) && !b.Start.Before(from.Add(-planLookback)) {
			planned = append(planned, breakAirings(b.ChannelID, b.Start, b.Pod)...)
		}
	}

	pol := a.pol()
	lengths := make([]time.Duration, len(plans))
	var fresh []StoredBreak
	keep := make([]time.Time, 0, len(plans)+len(stored))
	for _, b := range stored {
		if b.Start.Before(from) {
			keep = append(keep, b.Start) // behind the plan, within an evening: it aired, and counts
		}
	}
	now := a.now()
	t := from
	for i, plan := range plans {
		t = t.Add(plan.Gap)
		start := t
		if !bounds.Until.IsZero() && !start.Before(bounds.Until) {
			lengths[i] = plan.Ceiling // past this plan's end: the next window airs here, not this break
			t = t.Add(lengths[i])
			continue
		}
		sb, ok := byStart[start.UnixMilli()]
		if start.Before(bounds.FixedBefore) {
			// On air or aired: it stays as it aired, even if a clip has since left the catalog.
			lengths[i] = plan.Ceiling
			if ok {
				lengths[i] = sb.Length()
				keep = append(keep, start)
				planned = append(planned, breakAirings(channelID, start, sb.Pod)...)
			}
			t = t.Add(lengths[i])
			continue
		}
		// A stored break that found no ad is tried again before it airs: ads may have become ready, or
		// a rule that left it empty may have changed. It is replaced only when it now holds one.
		retry := ok && sb.Pod.MatchLevel == MatchBumperCard
		if !ok || retry || sb.Ceiling != plan.Ceiling || !podInCatalog(sb.Pod, inCatalog) {
			h := AiringHistory{Airings: append(append([]Airing(nil), history.Airings...), planned...), Settle: history.Settle}
			seed := start.UnixMilli()
			if seedAt != nil {
				seed = seedAt(start.UnixMilli())
			}
			assembled := a.assemble(clips, &h, channelID,
				Break{Seed: seed, Start: start, Length: plan.Ceiling}, sel, pol)
			sameEmpty := retry && sb.Ceiling == plan.Ceiling && podInCatalog(sb.Pod, inCatalog) &&
				assembled.MatchLevel == MatchBumperCard
			if !sameEmpty {
				sb = StoredBreak{ChannelID: channelID, Start: start, Ceiling: plan.Ceiling, Pod: assembled, PlannedAt: now}
				fresh = append(fresh, sb)
			}
		}
		lengths[i] = sb.Length()
		keep = append(keep, start)
		planned = append(planned, breakAirings(channelID, start, sb.Pod)...)
		t = t.Add(lengths[i])
	}
	if err := a.breaks.SaveBreaks(ctx, fresh); err != nil {
		return nil, err
	}
	if err := a.breaks.KeepBreaks(ctx, channelID, bounds.ClearFrom, bounds.Until, keep); err != nil {
		return nil, err
	}
	return lengths, nil
}

// PlannedLengths is the read side of PlanBreaks: each break's stored length where one is stored at
// its start for its ceiling, and its slot where none is. It assembles and stores nothing, so the
// playout can walk a window reconcile planned ahead without writing.
func (a *Adapter) PlannedLengths(ctx context.Context, channelID string, from time.Time, plans []BreakPlan) ([]time.Duration, error) {
	if a.breaks == nil || len(plans) == 0 {
		return nil, nil
	}
	horizon := from
	for _, p := range plans {
		horizon = horizon.Add(p.Gap + p.Ceiling)
	}
	stored, err := a.breaks.LoadBreaks(ctx, channelID, from, horizon)
	if err != nil {
		return nil, err
	}
	byStart := make(map[int64]StoredBreak, len(stored))
	for _, b := range stored {
		byStart[b.Start.UnixMilli()] = b
	}
	lengths := make([]time.Duration, len(plans))
	t := from
	for i, plan := range plans {
		t = t.Add(plan.Gap)
		lengths[i] = plan.Ceiling
		if sb, ok := byStart[t.UnixMilli()]; ok && sb.Ceiling == plan.Ceiling {
			lengths[i] = sb.Length()
		}
		t = t.Add(lengths[i])
	}
	return lengths, nil
}

// airWindow is how a clip airs now: where it starts in its file and for how long (#3384).
type airWindow struct{ startMs, lengthMs int64 }

// podInCatalog reports whether every real clip of a stored pod is still in the catalog and still
// airs as the pod booked it. A clip whose air window was measured after the break was planned
// makes the break plan again, or its dead air would keep airing from the stored break.
func podInCatalog(p Pod, inCatalog map[string]airWindow) bool {
	for _, e := range p.Entries {
		if e.Path == "" {
			continue
		}
		w, ok := inCatalog[e.Hash]
		if !ok || w.startMs != e.AirStartMs || w.lengthMs != e.DurationMs {
			return false
		}
	}
	return true
}

// breakAirings is a planned break's clips as the airings they will be: each at its start in the
// break, recorded long enough ago that the next break's snapshot counts it.
func breakAirings(channelID string, start time.Time, p Pod) []Airing {
	var out []Airing
	at := start
	for _, e := range p.Entries {
		if e.Path == "" || e.DurationMs <= 0 {
			continue
		}
		out = append(out, Airing{ChannelID: channelID, ClipHash: e.Hash, AiredAt: at, RecordedAt: at.Add(-24 * time.Hour)})
		at = at.Add(time.Duration(e.DurationMs) * time.Millisecond)
	}
	return out
}

// storedPods reads the stored pod of each break that has one, keyed by its start in Unix
// milliseconds. A read failure is not fatal: the breaks are assembled live.
func (a *Adapter) storedPods(ctx context.Context, channelID string, breaks []Break) map[int64]Pod {
	if a.breaks == nil {
		return nil
	}
	var lo, hi time.Time
	for _, b := range breaks {
		if b.Start.IsZero() || b.yearRound {
			continue
		}
		if lo.IsZero() || b.Start.Before(lo) {
			lo = b.Start
		}
		if b.Start.After(hi) {
			hi = b.Start
		}
	}
	if lo.IsZero() {
		return nil
	}
	stored, err := a.breaks.LoadBreaks(ctx, channelID, lo, hi)
	if err != nil {
		if a.log != nil {
			a.log.Warn("stored breaks unreadable (breaks are assembled live)", "channel", channelID, "err", err)
		}
		return nil
	}
	out := make(map[int64]Pod, len(stored))
	for _, b := range stored {
		out[b.Start.UnixMilli()] = b.Pod
	}
	return out
}
