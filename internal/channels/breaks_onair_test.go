package channels_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/testkit"
)

// The hold never outlasts the operator's break length: lowering it is an edit, and every break,
// aired or not, takes the shorter length at once (the stream may restart once). Only a catalog
// re-size is held.
func TestReconcile_ShorterBreakSettingIsNotHeld(t *testing.T) {
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
	for i, s := range got.Desired {
		if s.Kind == schedule.SlotFiller && s.DurationMs > time.Minute.Milliseconds() {
			t.Fatalf("break %d is %d ms after the break length was lowered to 1m", i, s.DurationMs)
		}
	}
}

// The household's 2026-10-24 evening, reduced: a channel on air for hours, whose breaks the next
// reconcile sizes differently. Before the fix, every break on the channel took the new length,
// the timeline walked from the anchor moved, and the stream was stopped: the viewer re-tuned into
// the programme they had already seen, or into the same break again. Now the breaks that aired
// keep their length, what is on air does not move, and the stream keeps playing.
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

	if !after.PlayoutAnchor.Equal(before.PlayoutAnchor) {
		t.Fatalf("anchor moved from %v to %v", before.PlayoutAnchor, after.PlayoutAnchor)
	}
	was := playout.AiringAt(before.Desired, before.PlayoutAnchor, now)
	is := playout.AiringAt(after.Desired, after.PlayoutAnchor, now)
	if was != is {
		t.Fatalf("on air moved: was %q at %v, now %q at %v", was.Title, was.Offset, is.Title, is.Offset)
	}
	if got := len(stops.StoppedChannels()); got != stopsBefore {
		t.Fatalf("re-sized breaks stopped the stream (%d stops)", got-stopsBefore)
	}
	var aired, resized int
	for _, s := range after.Desired {
		if s.Kind != schedule.SlotFiller {
			continue
		}
		switch s.DurationMs {
		case 113_366:
			aired++
		case 113_866:
			resized++
		}
	}
	// The breaks before the freeze horizon (now + 30 min) aired or are about to; those after it have
	// not, and take the catalog's new length.
	if aired != 2 || resized == 0 {
		t.Fatalf("breaks kept/re-sized = %d/%d, want the two that aired kept and the rest re-sized", aired, resized)
	}
}
