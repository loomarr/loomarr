package channels

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// BreakPlanner is the part of the pod assembler that plans a channel's breaks ahead (#3838 step 2):
// it assembles and stores each break once and reports its real length. Optional on a PodFiller; one
// that does not plan leaves the break slots at the length they have.
type BreakPlanner interface {
	PlanBreaks(ctx context.Context, channelID string, sel pod.Selection, seedAt func(startMs int64) int64,
		bounds pod.PlanBounds, plans []pod.BreakPlan) ([]time.Duration, error)
	// PlannedLengths reads what PlanBreaks stored, assembling and storing nothing.
	PlannedLengths(ctx context.Context, channelID string, from time.Time, plans []pod.BreakPlan) ([]time.Duration, error)
}

// The pod assembler plans; the engine finds that out by assertion, so a signature that drifts
// would quietly stop planning without this.
var _ BreakPlanner = (*pod.Adapter)(nil)

// nextWindowLead is how far ahead of a rolling window's turn reconcile plans the next window's
// breaks. Reconcile runs every few minutes, so an hour gives several chances; planning further
// ahead would assemble against an evening that has not happened.
const nextWindowLead = time.Hour

// TurnedWindow is what a channel airs once its rolling window has turned, before reconcile commits
// the new window: the cycle CyclePreview arranges at `at`, walked from the turn's anchor, with each
// break at the length reconcile planned for it ahead of the turn (WithPlannedBreaks).
func (e *Engine) TurnedWindow(ctx context.Context, channelID string, at, turnAnchor time.Time) ([]schedule.Slot, error) {
	_, slots, _, _, err := e.CyclePreview(ctx, channelID, at)
	if err != nil {
		return nil, err
	}
	return e.WithPlannedBreaks(ctx, channelID, turnAnchor, slots), nil
}

// WithPlannedBreaks is an arranged window walked from turnAnchor with each break at the length
// reconcile planned for it ahead of the turn (planNextWindow). It reads only and returns a new
// slice, so a cached arrangement passed in stays as it was: a break with nothing planned at its
// start keeps its slot and is assembled live, as before.
func (e *Engine) WithPlannedBreaks(ctx context.Context, channelID string, turnAnchor time.Time, slots []schedule.Slot) []schedule.Slot {
	planner, ok := e.pods.(BreakPlanner)
	if !ok {
		return slots
	}
	first, from, plans := breakPlans(turnAnchor, slots)
	if first < 0 {
		return slots
	}
	lengths, err := planner.PlannedLengths(ctx, channelID, from, plans)
	if err != nil {
		return slots // unplanned breaks air at their slot, assembled live
	}
	return withLengths(slots, first, lengths)
}

// planNextWindow plans the breaks of the window after the committed one, from the instant it will
// start airing, once that is within nextWindowLead. The playout airs the next window as soon as the
// programme crossing the boundary ends, before reconcile commits it, and reads these lengths
// (TurnedWindow). It clears only from the turn, so the window on air keeps its own breaks.
// Best-effort: a channel whose next window is not planned airs its breaks live, as before.
func (e *Engine) planNextWindow(ctx context.Context, ch channelstore.Channel) {
	if _, ok := e.pods.(BreakPlanner); !ok || len(ch.Desired) == 0 || ch.PlayoutAnchor.IsZero() {
		return
	}
	end, ok := e.turnEnd(ctx, ch, ch.PlayoutAnchor, ch.Desired)
	if !ok || !end.After(e.now()) || end.Sub(e.now()) > nextWindowLead {
		return
	}
	r, err := e.PreviewPlannedChannel(ctx, ch, end, e.avail)
	if err != nil {
		e.log.Warn("channels: next window's breaks not planned", "channel", ch.ID, "err", err)
		return
	}
	e.planBreaks(ctx, ch, end, r.Slots, time.Time{}, end)
}

// turnEnd is when the window walked from anchor stops airing: the end of the programme crossing its
// boundary, where the next window starts (#1675). False when the channel's window never turns there.
func (e *Engine) turnEnd(ctx context.Context, ch channelstore.Channel, anchor time.Time, slots []schedule.Slot) (time.Time, bool) {
	window, zone := e.RollingWindow(ch.Policy, e.now())
	if window <= 0 || anchor.IsZero() || len(slots) == 0 {
		return time.Time{}, false
	}
	if carries, err := e.CarriesOver(ctx, ch.Policy); err != nil || !carries {
		return time.Time{}, false
	}
	return timeline.CarryOverEnd(slots, anchor, schedule.NextWindowStart(anchor, window, zone)), true
}

// isCommercialBreak is a break slot: a filler slot with no key. A keyed filler slot stands in for a
// programme that is missing and keeps that programme's runtime.
func isCommercialBreak(s schedule.Slot) bool { return s.Kind == schedule.SlotFiller && s.Key == "" }

// breakPlans is the slots' breaks as plans from the first one: its index (-1 when there is none),
// its start walked from anchor, and each break's gap and slot length.
func breakPlans(anchor time.Time, slots []schedule.Slot) (first int, from time.Time, plans []pod.BreakPlan) {
	first, from = -1, anchor
	for i, s := range slots {
		if isCommercialBreak(s) {
			first = i
			break
		}
		from = from.Add(time.Duration(s.DurationMs) * time.Millisecond)
	}
	if first < 0 {
		return first, from, nil
	}
	var gap time.Duration
	for _, s := range slots[first:] {
		d := time.Duration(s.DurationMs) * time.Millisecond
		if isCommercialBreak(s) {
			plans = append(plans, pod.BreakPlan{Gap: gap, Ceiling: d})
			gap = 0
			continue
		}
		gap += d
	}
	return first, from, plans
}

// withLengths cuts each break from `first` on to its length, dropping a break with nothing to air.
// Slots it cannot match (a planner that returned the wrong count) come back as they were.
func withLengths(slots []schedule.Slot, first int, lengths []time.Duration) []schedule.Slot {
	if first < 0 {
		return slots
	}
	n := 0
	for _, s := range slots[first:] {
		if isCommercialBreak(s) {
			n++
		}
	}
	if len(lengths) != n {
		return slots
	}
	out := append([]schedule.Slot(nil), slots[:first]...)
	i := 0
	for _, s := range slots[first:] {
		if isCommercialBreak(s) {
			length := lengths[i]
			i++
			if length <= 0 {
				continue
			}
			s.DurationMs = length.Milliseconds()
		}
		out = append(out, s)
	}
	return out
}

// planBreaks cuts every break slot the playout has not started to the real length of its stored
// pod, so the programme after a break resumes when the break's own clips end. A break on air or
// aired is fixed: it keeps the length it was stored with, or its slot when it never was planned, so
// planning at any instant walks the timeline that aired. A break with nothing to air takes no time
// and is dropped. It returns the slots unchanged when the pod assembler does not plan.
//
// until and clearFrom bound the plan (pod.PlanBounds): the window on air stops at its turn and clears
// everything behind it; the next window starts at the turn and clears only from there.
func (e *Engine) planBreaks(ctx context.Context, ch channelstore.Channel, anchor time.Time, slots []schedule.Slot,
	until, clearFrom time.Time) []schedule.Slot {
	planner, ok := e.pods.(BreakPlanner)
	if !ok || len(slots) == 0 {
		return slots
	}
	if anchor.IsZero() {
		anchor = e.now().UTC().Truncate(time.Second)
	}
	first, from, plans := breakPlans(anchor, slots)
	if first < 0 {
		return slots
	}
	// A break within the run-ahead may already be resolving live, so it is fixed too. Whether a
	// break has started is judged on the planned timeline, which the stored lengths decide, so the
	// planner judges it.
	bounds := pod.PlanBounds{From: from, FixedBefore: e.now().Add(timeline.OnAirLookahead), Until: until, ClearFrom: clearFrom}
	lengths, err := planner.PlanBreaks(ctx, ch.ID, SelectionForChannel(ch), func(ms int64) int64 { return PodSeedAt(ch.ID, ms) },
		bounds, plans)
	if err != nil {
		return slots // unplanned breaks air as they are, assembled live
	}
	return withLengths(slots, first, lengths)
}

// planWindowOnAir plans the breaks of the cycle reconcile is about to commit, up to its turn: the
// breaks past the turn never air, and the next window's own plan owns that stretch. The turn is
// estimated from the slots as they stand; a break the estimate misplaces airs live, and the
// committed, planned cycle gives the next reconcile the exact turn.
func (e *Engine) planWindowOnAir(ctx context.Context, ch channelstore.Channel, anchor time.Time, slots []schedule.Slot) []schedule.Slot {
	if anchor.IsZero() {
		anchor = e.now().UTC().Truncate(time.Second)
	}
	until, _ := e.turnEnd(ctx, ch, anchor, slots)
	return e.planBreaks(ctx, ch, anchor, slots, until, time.Time{})
}
