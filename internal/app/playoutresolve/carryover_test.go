package playoutresolve

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// Rolling-window carry-over (#1675).
//
// Production shape, shrunk: every window arranges its own slice of 22-minute episodes and the
// slice is a little longer than the window, so one episode always crosses the boundary. Before
// the fix the next slice was walked from the channel's fixed anchor, so it was entered mid-phase:
// the crossing episode was cut and the next one joined part-way through. Live on the demo library
// a 22-minute episode aired 16 minutes, then the next slice joined another episode 8 minutes in.

// windowDecks arranges episodes per rolling window, named for the window: three 22-minute ones by
// default. With breakLen set, a commercial break follows each episode. planned holds the breaks
// reconcile stored ahead of a turn by their start, as the pod assembler does: a break airs its
// planned length only where the walk from the anchor reaches a stored start exactly.
type windowDecks struct {
	window       time.Duration
	zone         *time.Location
	episodes     int           // per window; 0 means 3
	episode      time.Duration // 0 means 22 minutes
	breakLen     time.Duration // the arranged break after each episode; 0 means none
	planned      map[int64]time.Duration
	label        string // the window opening's layout in episode names; "" means "15h"
	fixed        bool   // the channel cannot carry over (Tunarr-backed)
	asked        []time.Time
	turns        []time.Time // the anchors TurnedWindow was asked to plan from
	plannedAsked []time.Time // the anchors WithPlannedBreaks was asked to walk from
}

// planNextWindow stores the breaks of the window after the one committed at w0, each `length`
// long, from the instant that window starts airing: what reconcile's planNextWindow stores.
func (d *windowDecks) planNextWindow(w0 time.Time, length time.Duration) {
	ctx := context.Background()
	_, committed, _, _, _ := d.CyclePreview(ctx, "ch1", w0)
	turn := timeline.CarryOverEnd(committed, w0, schedule.NextWindowStart(w0, d.window, d.zone))
	_, next, _, _, _ := d.CyclePreview(ctx, "ch1", turn)
	d.planned = map[int64]time.Duration{}
	t := turn
	for _, s := range next {
		if s.Kind == schedule.SlotFiller && s.Key == "" {
			d.planned[t.UnixMilli()] = length
			t = t.Add(length)
			continue
		}
		t = t.Add(time.Duration(s.DurationMs) * time.Millisecond)
	}
}

func (d *windowDecks) TurnedWindow(ctx context.Context, id string, at, turnAnchor time.Time) ([]schedule.Slot, error) {
	d.turns = append(d.turns, turnAnchor)
	_, slots, _, _, err := d.CyclePreview(ctx, id, at)
	if err != nil {
		return nil, err
	}
	return d.WithPlannedBreaks(ctx, id, turnAnchor, slots), nil
}

func (d *windowDecks) WithPlannedBreaks(_ context.Context, _ string, turnAnchor time.Time, slots []schedule.Slot) []schedule.Slot {
	d.plannedAsked = append(d.plannedAsked, turnAnchor)
	out := append([]schedule.Slot(nil), slots...)
	t := turnAnchor
	for i := range out {
		if out[i].Kind == schedule.SlotFiller && out[i].Key == "" {
			if l, ok := d.planned[t.UnixMilli()]; ok {
				out[i].DurationMs = l.Milliseconds()
			}
		}
		t = t.Add(time.Duration(out[i].DurationMs) * time.Millisecond)
	}
	return out
}

func (d *windowDecks) CyclePreview(_ context.Context, _ string, at time.Time) (
	time.Time, []schedule.Slot, schedule.ActiveRuleAttribution, time.Duration, error,
) {
	d.asked = append(d.asked, at)
	opened := schedule.WindowStart(at, d.window, d.zone)
	episodes, episode, label := d.episodes, d.episode, d.label
	if episodes == 0 {
		episodes = 3
	}
	if episode == 0 {
		episode = 22 * time.Minute
	}
	if label == "" {
		label = "15h"
	}
	var slots []schedule.Slot
	for ep := 1; ep <= episodes; ep++ {
		id := fmt.Sprintf("%s-e%d", opened.Format(label), ep)
		slots = append(slots, schedule.Slot{
			Kind: schedule.SlotProgram, Key: provision.Key("series:tvdb:90000001"), Title: id, LibraryItemID: id,
			DurationMs: episode.Milliseconds(),
		})
		if d.breakLen > 0 {
			slots = append(slots, schedule.Slot{Kind: schedule.SlotFiller, DurationMs: d.breakLen.Milliseconds()})
		}
	}
	return at, slots, schedule.ActiveRuleAttribution{}, d.window, nil
}

func (d *windowDecks) RollingWindow(schedule.ChannelPolicy, time.Time) (time.Duration, *time.Location) {
	return d.window, d.zone
}

func (d *windowDecks) Calendar() schedule.Calendar { return schedule.Calendar{} }

func (d *windowDecks) CarriesOver(context.Context, schedule.ChannelPolicy) (bool, error) {
	return !d.fixed, nil
}

// carryOverResolver has window 00:00–01:00 committed: its slice, walked from the anchor 00:00.
func carryOverResolver(now time.Time) (*Resolver, time.Time) {
	w0 := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)
	decks := &windowDecks{window: time.Hour}
	_, committed, _, _, _ := decks.CyclePreview(context.Background(), "ch1", w0)
	accepted := &stubChannels{}
	accepted.ch.ID, accepted.ch.Desired, accepted.ch.PlayoutAnchor = "ch1", committed, w0
	accepted.ch.Status = schedule.StatusLive
	return &Resolver{engine: decks, channels: accepted, now: func() time.Time { return now }}, w0
}

// A programme spanning the boundary airs whole, and the next window starts at its end — in the
// guide, with no clipped block and no overlap.
func TestGuide_ProgrammeCrossingTheWindowBoundaryAirsWhole(t *testing.T) {
	t.Parallel()
	r, w0 := carryOverResolver(time.Date(2026, time.August, 15, 0, 30, 0, 0, time.UTC))

	bs, err := r.segmentedBroadcasts(context.Background(), "ch1", w0, w0.Add(3*time.Hour), timeline.BroadcastsBetween)
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range bs {
		if b.Duration() != 22*time.Minute {
			t.Errorf("%s aired %v of 22m (%s → %s): a programme was cut", b.Title, b.Duration(), b.Start.Format("15:04"), b.Stop.Format("15:04"))
		}
		if i > 0 && !b.Start.Equal(bs[i-1].Stop) {
			t.Errorf("%s starts %s but %s stops %s: the timeline is not continuous",
				b.Title, b.Start.Format("15:04"), bs[i-1].Title, bs[i-1].Stop.Format("15:04"))
		}
	}
	// 00:44–01:06 is the last episode of the 00h slice; the 01h slice starts where it ends.
	if len(bs) < 4 || bs[2].Title != "00h-e3" || bs[3].Title != "01h-e1" || !bs[3].Start.Equal(w0.Add(66*time.Minute)) {
		t.Fatalf("handoff wrong: %v", guideLine(bs))
	}
}

// A window that began with a carried-over programme starts at that programme's end, so the stretch
// between the window's opening and its anchor belongs to the window before. The guide read from
// the opening must show the carried programme there, not a gap (found live: a guide from 00:00
// was empty until the anchor at 00:05).
func TestGuide_WindowOpenedByACarryOverShowsTheCarriedProgramme(t *testing.T) {
	t.Parallel()
	w0 := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)
	decks := &windowDecks{window: time.Hour}
	_, committed, _, _, _ := decks.CyclePreview(context.Background(), "ch1", w0.Add(time.Hour))
	accepted := &stubChannels{}
	accepted.ch.ID, accepted.ch.Desired, accepted.ch.PlayoutAnchor = "ch1", committed, w0.Add(66*time.Minute)
	accepted.ch.Status = schedule.StatusLive
	r := &Resolver{engine: decks, channels: accepted, now: func() time.Time { return w0.Add(90 * time.Minute) }}

	bs, err := r.segmentedBroadcasts(context.Background(), "ch1", w0.Add(time.Hour), w0.Add(2*time.Hour), timeline.BroadcastsBetween)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) < 2 || bs[0].Title != "00h-e3" || !bs[0].Start.Equal(w0.Add(44*time.Minute)) || !bs[0].Stop.Equal(w0.Add(66*time.Minute)) || bs[1].Title != "01h-e1" {
		t.Fatalf("guide from the window's opening: %v, want the carried 00h-e3 00:44–01:06, then 01h-e1", guideLine(bs))
	}
}

// The encoder walks the same timeline: the crossing episode keeps airing past the boundary, and
// the next window's first episode starts at its end — before any reconcile turns the window.
func TestAiringAt_CarriesTheCrossingProgrammeOverTheBoundary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		at         time.Duration // after 00:00
		want       string
		wantOffset time.Duration
	}{
		{at: 61 * time.Minute, want: "00h-e3", wantOffset: 17 * time.Minute},
		{at: 66*time.Minute + time.Second, want: "01h-e1", wantOffset: time.Second},
	} {
		r, w0 := carryOverResolver(w0At(tc.at))
		slots, epoch, err := r.acceptedCycle(context.Background(), "ch1", w0.Add(tc.at))
		if err != nil {
			t.Fatal(err)
		}
		got := timeline.AiringAt(slots, epoch, w0.Add(tc.at))
		if got.LibraryItemID != tc.want || got.Offset != tc.wantOffset {
			t.Errorf("at +%v: airing %s at offset %v, want %s at %v", tc.at, got.LibraryItemID, got.Offset, tc.want, tc.wantOffset)
		}
	}
}

// Past the turn, the next window airs with its breaks planned from the turn's anchor, the anchor
// reconcile will commit: a planned break airs its stored pod and ends when the pod does, rather than
// airing its full slot with the break card filling the rest until reconcile runs (#3838 step 2).
func TestAiringAt_ATurnedWindowIsPlannedFromTheTurn(t *testing.T) {
	t.Parallel()
	r, w0 := carryOverResolver(w0At(70 * time.Minute))
	if _, _, err := r.acceptedCycle(context.Background(), "ch1", w0.Add(70*time.Minute)); err != nil {
		t.Fatal(err)
	}
	turns := r.engine.(*windowDecks).turns
	if len(turns) != 1 || !turns[0].Equal(w0.Add(66*time.Minute)) {
		t.Fatalf("turned window planned from %v, want once from the turn at 01:06", turns)
	}
}

// deckResolver has the window opening at w0 committed: its slice, walked from the anchor w0.
func deckResolver(decks *windowDecks, w0, now time.Time) *Resolver {
	_, committed, _, _, _ := decks.CyclePreview(context.Background(), "ch1", w0)
	accepted := &stubChannels{}
	accepted.ch.ID, accepted.ch.Desired, accepted.ch.PlayoutAnchor = "ch1", committed, w0
	accepted.ch.Status = schedule.StatusLive
	return &Resolver{engine: decks, channels: accepted, now: func() time.Time { return now }}
}

// guideAgreesWithAir reads the guide over [from, to) at the resolver's now and checks that every
// programme the guide starts before `until` is what playout airs one second after that start.
func guideAgreesWithAir(t *testing.T, r *Resolver, from, to, until time.Time) {
	t.Helper()
	bs, err := r.segmentedBroadcasts(context.Background(), "ch1", from, to, timeline.BroadcastsBetween)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, b := range bs {
		if b.Kind != schedule.SlotProgram || !b.Start.Before(until) {
			continue
		}
		checked++
		at := b.Start.Add(time.Second)
		slots, epoch, err := r.acceptedCycle(context.Background(), "ch1", at)
		if err != nil {
			t.Fatal(err)
		}
		got := timeline.AiringAt(slots, epoch, at)
		if got.LibraryItemID != b.LibraryItemID || got.Offset != time.Second {
			t.Errorf("guide starts %s at %s; playout airs %s %v in at that instant (the guide is %v late)",
				b.Title, b.Start.Format(time.DateTime), got.LibraryItemID, got.Offset-time.Second, got.Offset-time.Second)
		}
	}
	if checked < 4 {
		t.Fatalf("checked %d programmes, want the turn covered: %v", checked, guideLine(bs))
	}
}

// Read before the turn, the guide's next window is the one playout airs after it, breaks at the
// lengths reconcile planned for them (#4551). Each 5-minute break is planned at 2 minutes, so
// forecasting the arranged lengths put every programme after the first break late in the guide.
func TestGuide_NextWindowAirsWithItsPlannedBreaks(t *testing.T) {
	t.Parallel()
	w0 := w0At(0)
	decks := &windowDecks{window: time.Hour, breakLen: 5 * time.Minute}
	decks.planNextWindow(w0, 2*time.Minute)
	r := deckResolver(decks, w0, w0.Add(30*time.Minute))
	guideAgreesWithAir(t, r, w0, w0.Add(3*time.Hour), w0.Add(2*time.Hour))
}

// Only the window after the one on air is planned ahead, so only it is walked for planned breaks,
// from its own turn; the window after that is listed at its arranged break lengths.
func TestGuide_WindowAfterTheNextKeepsItsArrangedBreaks(t *testing.T) {
	t.Parallel()
	w0 := w0At(0)
	decks := &windowDecks{window: time.Hour, breakLen: 5 * time.Minute}
	decks.planNextWindow(w0, 2*time.Minute)
	r := deckResolver(decks, w0, w0.Add(30*time.Minute))
	bs, err := r.segmentedBroadcasts(context.Background(), "ch1", w0, w0.Add(4*time.Hour), timeline.BroadcastsBetween)
	if err != nil {
		t.Fatal(err)
	}
	// 00h-e3 ends at 01:16, the turn; 01h's breaks are planned from there.
	if turn := w0.Add(76 * time.Minute); len(decks.plannedAsked) != 1 || !decks.plannedAsked[0].Equal(turn) {
		t.Fatalf("planned breaks read from %v, want once from the turn at %v", decks.plannedAsked, turn)
	}
	var last *timeline.Broadcast
	gaps := 0
	for i, b := range bs {
		if b.Kind != schedule.SlotProgram || b.Title[:3] != "02h" {
			continue
		}
		if last != nil {
			gaps++
			if gap := b.Start.Sub(last.Stop); gap != 5*time.Minute {
				t.Errorf("02h break before %s is %v, want the arranged 5m", b.Title, gap)
			}
		}
		last = &bs[i]
	}
	if gaps < 2 {
		t.Fatalf("the guide does not cover the 02h window: %v", guideLine(bs))
	}
}

// With nothing planned, the next window's breaks keep their arranged length on both sides.
func TestGuide_NextWindowWithNoPlannedBreaksAirsArranged(t *testing.T) {
	t.Parallel()
	w0 := w0At(0)
	decks := &windowDecks{window: time.Hour, breakLen: 5 * time.Minute}
	r := deckResolver(decks, w0, w0.Add(30*time.Minute))
	guideAgreesWithAir(t, r, w0, w0.Add(3*time.Hour), w0.Add(2*time.Hour))
}

// A day window in a DST zone is 25 hours on the night the clocks go back; the planned next day
// still lines up with what playout airs.
func TestGuide_NextWindowAirsWithItsPlannedBreaksAcrossADSTDay(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	w0 := time.Date(2026, time.November, 1, 0, 0, 0, 0, zone) // 25 hours to the next midnight
	decks := &windowDecks{
		window: 24 * time.Hour, zone: zone, episodes: 24, episode: time.Hour, label: "Jan02",
		breakLen: 5 * time.Minute,
	}
	decks.planNextWindow(w0, 2*time.Minute)
	nextMidnight := time.Date(2026, time.November, 2, 0, 0, 0, 0, zone)
	// The episode crossing midnight ends at 00:55, the turn; reconcile plans within the hour before.
	r := deckResolver(decks, w0, nextMidnight.Add(30*time.Minute))
	if got := nextMidnight.Sub(w0); got != 25*time.Hour {
		t.Fatalf("window length %v, want the 25-hour day", got)
	}
	guideAgreesWithAir(t, r, w0, nextMidnight.Add(48*time.Hour), nextMidnight.Add(24*time.Hour))
}

// A channel that cannot carry over never turns ahead of reconcile, so planned breaks do not move
// its guide.
func TestGuide_FixedAnchorChannelIgnoresPlannedBreaks(t *testing.T) {
	t.Parallel()
	w0 := w0At(0)
	read := func(planned time.Duration) []string {
		decks := &windowDecks{window: time.Hour, breakLen: 5 * time.Minute, fixed: true}
		if planned > 0 {
			decks.planNextWindow(w0, planned)
		}
		r := deckResolver(decks, w0, w0.Add(30*time.Minute))
		bs, err := r.segmentedBroadcasts(context.Background(), "ch1", w0, w0.Add(3*time.Hour), timeline.BroadcastsBetween)
		if err != nil {
			t.Fatal(err)
		}
		return guideLine(bs)
	}
	arranged, planned := read(0), read(2*time.Minute)
	if fmt.Sprint(arranged) != fmt.Sprint(planned) {
		t.Fatalf("fixed-anchor guide moved with planned breaks:\n%v\nwant\n%v", planned, arranged)
	}
}

func w0At(d time.Duration) time.Time {
	return time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC).Add(d)
}

func guideLine(bs []timeline.Broadcast) []string {
	var out []string
	for _, b := range bs {
		out = append(out, fmt.Sprintf("%s %s–%s", b.Title, b.Start.Format("15:04"), b.Stop.Format("15:04")))
	}
	return out
}
