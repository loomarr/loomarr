package channels

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// planningPods is a pod assembler that plans: each break gets the length it is told to, and it
// records what it was asked.
type planningPods struct {
	lengths []time.Duration
	plans   []pod.BreakPlan
	from    time.Time
	bounds  pod.PlanBounds
	reads   int
}

func (p *planningPods) PlannedLengths(_ context.Context, _ string, from time.Time, plans []pod.BreakPlan) ([]time.Duration, error) {
	p.reads++
	p.plans, p.from = plans, from
	return p.lengths, nil
}

var _ BreakPlanner = (*planningPods)(nil)

func (p *planningPods) BuildFillerList(context.Context, string, int64, pod.Selection) ([]string, bool) {
	return nil, false
}
func (p *planningPods) TunarrFillerIDs(context.Context, string, int64, pod.Selection) []string {
	return nil
}
func (p *planningPods) PlayableDurationMs(context.Context, string, int64, pod.Selection) int64 {
	return 120_000
}
func (p *planningPods) PlanBreaks(_ context.Context, _ string, _ pod.Selection, _ func(int64) int64,
	bounds pod.PlanBounds, plans []pod.BreakPlan) ([]time.Duration, error) {
	p.plans, p.from, p.bounds = plans, bounds.From, bounds
	return p.lengths, nil
}

func programme(id string, minutes int) schedule.Slot {
	return schedule.Slot{Kind: schedule.SlotProgram, LibraryItemID: id, DurationMs: int64(minutes) * 60_000, Segment: 1}
}

func breakSlot() schedule.Slot { return schedule.Slot{Kind: schedule.SlotFiller, DurationMs: 120_000} }

// A break with fewer fresh ads than its slot ends early, and the programme resumes at the real
// end of the break, as the playout walks the planned cycle: the break is cut to its stored length
// and the programme after it starts that much sooner.
func TestPlanBreaksResumesTheProgrammeAtTheRealEndOfTheBreak(t *testing.T) {
	anchor := time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC)
	now := anchor.Add(-time.Hour) // the whole cycle is still to come
	pods := &planningPods{lengths: []time.Duration{90 * time.Second, 5 * time.Second}}
	e := &Engine{pods: pods, now: func() time.Time { return now }}
	slots := []schedule.Slot{programme("a", 30), breakSlot(), programme("b", 30), breakSlot(), programme("c", 30)}

	got := e.planBreaks(context.Background(), channelstore.Channel{}, anchor, slots, time.Time{}, time.Time{})

	if len(pods.plans) != 2 || pods.plans[0].Gap != 0 || pods.plans[0].Ceiling != 2*time.Minute ||
		pods.plans[1].Gap != 30*time.Minute || pods.plans[1].Ceiling != 2*time.Minute {
		t.Fatalf("plans = %+v, want two breaks 30 minutes of programme apart, each with a 2 minute ceiling", pods.plans)
	}
	if want := anchor.Add(30 * time.Minute); !pods.from.Equal(want) {
		t.Fatalf("planning starts at %v, want the first break's start %v", pods.from, want)
	}
	if got[1].DurationMs != 90_000 || got[3].DurationMs != 5_000 {
		t.Fatalf("break lengths = %d, %d ms, want 90000 and 5000", got[1].DurationMs, got[3].DurationMs)
	}
	// Across the playout boundary: the second programme starts when the first break's clips end.
	at := anchor.Add(30*time.Minute + 90*time.Second)
	if a := timeline.AiringAt(got, anchor, at); a.LibraryItemID != "b" || a.Offset != 0 {
		t.Fatalf("at the end of the first break the channel airs %+v, want programme b from its start", a)
	}
	// And just before it, the break is still on.
	if a := timeline.AiringAt(got, anchor, at.Add(-time.Second)); a.Kind != schedule.SlotFiller {
		t.Fatalf("a second before the break ends the channel airs %+v, want the break", a)
	}
}

// A break with nothing to air takes no time. A break already on air or aired is planned too, as
// fixed: the planner keeps the length it aired at, which may be shorter than the fresh slot a
// turned window brings, and the programmes after it follow that length.
func TestPlanBreaksDropsAnEmptyBreakAndKeepsTheBreaksThatHaveStarted(t *testing.T) {
	anchor := time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC)
	now := anchor.Add(45 * time.Minute) // the first break aired, the second is still to come
	pods := &planningPods{lengths: []time.Duration{40 * time.Second, 0}}
	e := &Engine{pods: pods, now: func() time.Time { return now }}
	slots := []schedule.Slot{programme("a", 30), breakSlot(), programme("b", 30), breakSlot(), programme("c", 30)}

	got := e.planBreaks(context.Background(), channelstore.Channel{}, anchor, slots, time.Time{}, time.Time{})

	if len(pods.plans) != 2 || !pods.from.Equal(anchor.Add(30*time.Minute)) {
		t.Fatalf("plans = %+v from %v, want both breaks from the first", pods.plans, pods.from)
	}
	if want := now.Add(timeline.OnAirLookahead); !pods.bounds.FixedBefore.Equal(want) {
		t.Fatalf("fixed before %v, want now plus the run-ahead %v", pods.bounds.FixedBefore, want)
	}
	if len(got) != 4 || got[1].DurationMs != 40_000 {
		t.Fatalf("slots = %+v, want the aired break at the 40 seconds it aired and the empty one dropped", got)
	}
}

// An assembler that does not plan leaves the slots as they are.
func TestPlanBreaksLeavesSlotsAloneWithoutAPlanner(t *testing.T) {
	e := &Engine{pods: nil, now: time.Now}
	slots := []schedule.Slot{programme("a", 30), breakSlot()}
	if got := e.planBreaks(context.Background(), channelstore.Channel{}, time.Now(), slots, time.Time{}, time.Time{}); len(got) != 2 || got[1].DurationMs != 120_000 {
		t.Fatalf("slots = %+v, want them unchanged", got)
	}
}
