package channels

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout"
)

// BreakPlanner is the part of the pod assembler that plans a channel's breaks ahead (#3838 step 2):
// it assembles and stores each break once and reports its real length. Optional on a PodFiller; one
// that does not plan leaves the break slots at the length they have.
type BreakPlanner interface {
	PlanBreaks(ctx context.Context, channelID string, sel pod.Selection, seedAt func(startMs int64) int64,
		anchor, from time.Time, plans []pod.BreakPlan) ([]time.Duration, error)
}

// isCommercialBreak is a break slot: a filler slot with no key. A keyed filler slot stands in for a
// programme that is missing and keeps that programme's runtime.
func isCommercialBreak(s schedule.Slot) bool { return s.Kind == schedule.SlotFiller && s.Key == "" }

// planBreaks cuts every break slot the playout has not started to the real length of its stored
// pod, so the programme after a break resumes when the break's own clips end. The breaks before the
// first one still to come are fixed: they are on air or aired, and keep the length they have. A
// break with nothing to air takes no time and is dropped. It returns the slots unchanged when the
// pod assembler does not plan.
func (e *Engine) planBreaks(ctx context.Context, ch channelstore.Channel, anchor time.Time, slots []schedule.Slot) []schedule.Slot {
	planner, ok := e.pods.(BreakPlanner)
	if !ok || len(slots) == 0 {
		return slots
	}
	if anchor.IsZero() {
		anchor = e.now().UTC().Truncate(time.Second)
	}
	// A break within the run-ahead may already be resolving live, so only later ones are planned.
	until := e.now().Add(playout.OnAirLookahead)
	first, from := -1, anchor
	for i, s := range slots {
		if isCommercialBreak(s) && !from.Before(until) {
			first = i
			break
		}
		from = from.Add(time.Duration(s.DurationMs) * time.Millisecond)
	}
	if first < 0 {
		return slots
	}
	var plans []pod.BreakPlan
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
	lengths, err := planner.PlanBreaks(ctx, ch.ID, SelectionForChannel(ch), func(ms int64) int64 { return PodSeedAt(ch.ID, ms) },
		anchor, from, plans)
	if err != nil || len(lengths) != len(plans) {
		return slots // unplanned breaks air as they are, assembled live
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
