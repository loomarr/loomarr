package channels_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// recordingPlanner plans every break to 30 seconds, remembering each plan's bounds and the lengths
// it planned by start, which PlannedLengths reads back.
type recordingPlanner struct {
	plans   []pod.PlanBounds
	planned map[int64]time.Duration
}

func (p *recordingPlanner) BuildFillerList(context.Context, string, int64, pod.Selection) ([]string, bool) {
	return nil, false
}
func (p *recordingPlanner) HasPool(context.Context, string, int64, pod.Selection) bool { return true }
func (p *recordingPlanner) PlayableDurationMs(context.Context, string, int64, pod.Selection) int64 {
	return (10 * time.Minute).Milliseconds()
}

func (p *recordingPlanner) PlanBreaks(_ context.Context, _ string, _ pod.Selection, _ func(int64) int64,
	b pod.PlanBounds, plans []pod.BreakPlan) ([]time.Duration, error) {
	p.plans = append(p.plans, b)
	out := make([]time.Duration, len(plans))
	t := b.From
	for i, pl := range plans {
		t = t.Add(pl.Gap)
		out[i] = pl.Ceiling
		if b.Until.IsZero() || t.Before(b.Until) {
			out[i] = 30 * time.Second
			p.planned[t.UnixMilli()] = out[i]
		}
		t = t.Add(out[i])
	}
	return out, nil
}

func (p *recordingPlanner) PlannedLengths(_ context.Context, _ string, from time.Time, plans []pod.BreakPlan) ([]time.Duration, error) {
	out := make([]time.Duration, len(plans))
	t := from
	for i, pl := range plans {
		t = t.Add(pl.Gap)
		out[i] = pl.Ceiling
		if l, ok := p.planned[t.UnixMilli()]; ok {
			out[i] = l
		}
		t = t.Add(out[i])
	}
	return out, nil
}

var _ channels.BreakPlanner = (*recordingPlanner)(nil)

// Reconcile plans the next window's breaks within the hour before the window turns, from the turn
// and clearing only from it, and stops the window on air's plan at the turn. Past the turn, before
// any reconcile commits the new window, the playout reads those lengths: each break ends with its
// pod from the window's first break, and nothing is assembled on the read (#3838 step 2).
func TestReconcile_PlansTheNextWindowsBreaksBeforeItTurns(t *testing.T) {
	st := newStore(t)
	entries, avail := films(20, 100*time.Minute)
	opened := time.Unix(1_800_000_000, 0).UTC().Truncate(24 * time.Hour)
	now := opened.Add(3 * time.Hour)
	planner := &recordingPlanner{planned: map[int64]time.Duration{}}
	e := channels.New(st, nil, avail, nil, channels.Config{
		ReconcileTTL: 10 * time.Minute, DefaultWindow: 24 * time.Hour, BreaksPerHour: 2,
		ResolvePlayoutBackendContext: func(context.Context) (string, error) { return schedule.PlayoutBackendInternal, nil },
	}, func() time.Time { return now }, nil).WithPods(planner)
	seedShuffleChannel(t, st, "nw", entries)

	first := reconcileDesired(t, e, st, "nw")
	turn := timeline.CarryOverEnd(first.Desired, first.PlayoutAnchor, opened.Add(24*time.Hour))
	if !hasBreak(first.Desired) {
		t.Fatal("fixture: the channel has no breaks to plan")
	}
	for _, b := range planner.plans {
		if b.ClearFrom.Equal(turn) {
			t.Fatalf("the next window was planned %v before its turn, want only within the hour", turn.Sub(now))
		}
	}

	now = turn.Add(-30 * time.Minute)
	planner.plans = nil
	reconcileDesired(t, e, st, "nw")
	var onAir, next *pod.PlanBounds
	for i, b := range planner.plans {
		switch {
		case b.ClearFrom.IsZero():
			onAir = &planner.plans[i]
		case b.ClearFrom.Equal(turn):
			next = &planner.plans[i]
		}
	}
	if onAir == nil || !onAir.Until.Equal(turn) {
		t.Fatalf("plans %+v: want the window on air planned up to its turn at %v", planner.plans, turn)
	}
	if next == nil || next.From.Before(turn) || !next.Until.IsZero() {
		t.Fatalf("plans %+v: want the next window planned from its turn at %v", planner.plans, turn)
	}

	planner.plans = nil
	now = turn.Add(time.Minute)
	slots, err := e.TurnedWindow(context.Background(), "nw", now, turn)
	if err != nil {
		t.Fatal(err)
	}
	if len(planner.plans) != 0 {
		t.Fatalf("reading the turned window planned %d times; the playout only reads", len(planner.plans))
	}
	var breaks int
	for _, s := range slots {
		if s.Kind == schedule.SlotFiller && s.Key == "" {
			breaks++
			if s.DurationMs != (30 * time.Second).Milliseconds() {
				t.Fatalf("a turned window's break airs %v, want the 30 seconds planned ahead", time.Duration(s.DurationMs)*time.Millisecond)
			}
		}
	}
	if breaks == 0 {
		t.Fatal("the turned window has no breaks")
	}

	// The guide applies the planned lengths to its cached arrangement, which must come back as
	// it went in.
	_, arranged, _, _, err := e.CyclePreview(context.Background(), "nw", now)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]schedule.Slot(nil), arranged...)
	if got := e.WithPlannedBreaks(context.Background(), "nw", turn, arranged); !reflect.DeepEqual(got, slots) {
		t.Fatal("the planned arrangement differs from the turned window playout airs")
	}
	if !reflect.DeepEqual(arranged, before) {
		t.Fatal("applying the planned breaks changed the arrangement passed in")
	}
}

func hasBreak(slots []schedule.Slot) bool {
	for _, s := range slots {
		if s.Kind == schedule.SlotFiller && s.Key == "" {
			return true
		}
	}
	return false
}
