package channels_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/testkit"
)

// Lowering the break length is an edit (#4140 step 1): it applies at once from the programme after
// the one on air, and the breaks that aired, and the one after the programme on air, keep the
// length they aired with, so nothing on air moves.
func TestReconcile_ShorterBreakSettingAppliesAfterTheProgrammeOnAir(t *testing.T) {
	st := newStore(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	breakLen := 2 * time.Minute
	pods := &fakePods{ids: []string{"clip"}, duration: 113_366}
	e := channels.New(st, nil, hourAvail{
		"movie:tmdb:90000001": "lib-1", "movie:tmdb:90000002": "lib-2", "movie:tmdb:90000003": "lib-3",
	}, nil, channels.Config{
		ReconcileTTL: 10 * time.Minute, BreaksPerHour: 1,
		ResolveBreakDuration:         func() time.Duration { return breakLen },
		ResolvePlayoutBackendContext: func(context.Context) (string, error) { return schedule.PlayoutBackendInternal, nil },
	}, func() time.Time { return now }, testkit.Logger()).WithPods(pods)
	seedChannel(t, st, "shorter", 10,
		entry("movie:tmdb:90000001", "First"), entry("movie:tmdb:90000002", "Second"), entry("movie:tmdb:90000003", "Third"))
	if err := e.Reconcile(context.Background(), "shorter"); err != nil {
		t.Fatal(err)
	}

	now = now.Add(150 * time.Minute)
	breakLen, pods.duration = time.Minute, 58_000
	if err := e.Reconcile(context.Background(), "shorter"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetChannel(context.Background(), "shorter")
	if err != nil {
		t.Fatal(err)
	}
	onAir := playout.AiringAt(got.Desired, got.PlayoutAnchor, now)
	onAirEnds := now.Add(onAir.Remaining)
	shorter := 0
	for _, b := range playout.BroadcastsBetween(got.Desired, got.PlayoutAnchor, got.PlayoutAnchor, now.Add(12*time.Hour)) {
		if b.Kind != schedule.SlotFiller {
			continue
		}
		switch {
		case !b.Start.After(onAirEnds) && b.Duration() <= time.Minute:
			t.Errorf("the break at %v aired or follows the programme on air, and was shortened to %v", b.Start, b.Duration())
		case b.Start.After(onAirEnds) && b.Duration() > time.Minute:
			t.Errorf("the break at %v is %v after the break length was lowered to 1m", b.Start, b.Duration())
		case b.Start.After(onAirEnds):
			shorter++
		}
	}
	if shorter == 0 {
		t.Fatal("no break after the programme on air took the shorter length")
	}
}

// The household's 2026-10-24 evening, reduced: a channel on air for hours, whose breaks the next
// reconcile sizes differently. Before the fix, every break on the channel took the new length,
// the timeline walked from the anchor moved, and the stream was stopped: the viewer re-tuned into
// the programme they had already seen, or into the same break again. Now what aired and what is on
// air do not move, the new length applies after the programme on air, and the stream keeps playing.
func TestReconcile_ReSizedBreaksNeverMoveWhatIsOnAir(t *testing.T) {
	st := newStore(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	pods := &fakePods{ids: []string{"clip"}, duration: 113_366}
	e := channels.New(st, nil, hourAvail{
		"movie:tmdb:90000001": "lib-1", "movie:tmdb:90000002": "lib-2", "movie:tmdb:90000003": "lib-3",
		"movie:tmdb:90000004": "lib-4", "movie:tmdb:90000005": "lib-5", "movie:tmdb:90000006": "lib-6",
	}, nil, channels.Config{
		ReconcileTTL: 10 * time.Minute, BreaksPerHour: 1,
		ResolvePlayoutBackendContext: func(context.Context) (string, error) { return schedule.PlayoutBackendInternal, nil },
	}, func() time.Time { return now }, testkit.Logger()).WithPods(pods)
	stops := &testkit.Playout{}
	e.WithScheduleInvalidator(stops)
	seedChannel(t, st, "on-air", 9,
		entry("movie:tmdb:90000001", "First"), entry("movie:tmdb:90000002", "Second"), entry("movie:tmdb:90000003", "Third"),
		entry("movie:tmdb:90000004", "Fourth"), entry("movie:tmdb:90000005", "Fifth"), entry("movie:tmdb:90000006", "Sixth"))
	if err := e.Reconcile(context.Background(), "on-air"); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetChannel(context.Background(), "on-air")
	if err != nil {
		t.Fatal(err)
	}

	// Two and a half hours on, two breaks have aired and the third film is on air. The catalog
	// changed, so the pod is half a second longer.
	now = now.Add(150 * time.Minute)
	stopsBefore := len(stops.StoppedChannels())
	pods.duration = 113_866
	if err := e.Reconcile(context.Background(), "on-air"); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetChannel(context.Background(), "on-air")
	if err != nil {
		t.Fatal(err)
	}

	// This channel's window never turns, so the catalog change re-deals after the programme on air:
	// the third film and the break after it keep the times they air at, and the cycle starts again
	// at the film, so the breaks that aired are behind the anchor and are never walked again.
	was := playout.AiringAt(before.Desired, before.PlayoutAnchor, now)
	is := playout.AiringAt(after.Desired, after.PlayoutAnchor, now)
	if was != is {
		t.Fatalf("on air moved: was %q at %v, now %q at %v", was.Title, was.Offset, is.Title, is.Offset)
	}
	if want := now.Add(-was.Offset); !after.PlayoutAnchor.Equal(want) {
		t.Fatalf("anchor = %v, want the start of the film on air %v", after.PlayoutAnchor, want)
	}
	if got := len(stops.StoppedChannels()); got != stopsBefore {
		t.Fatalf("re-sized breaks stopped the stream (%d stops)", got-stopsBefore)
	}
	var kept, resized int
	for _, s := range after.Desired {
		if s.Kind != schedule.SlotFiller {
			continue
		}
		switch s.DurationMs {
		case 113_366:
			kept++
		case 113_866:
			resized++
		}
	}
	if kept != 1 || resized == 0 {
		t.Fatalf("breaks kept/re-sized = %d/%d, want the one after the film on air kept and the rest re-sized", kept, resized)
	}
}
