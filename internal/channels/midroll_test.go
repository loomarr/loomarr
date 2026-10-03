package channels_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tunarr/tunarrtest"
)

// hourAvail resolves every key to a one-hour library item.
type hourAvail map[provision.Key]string

func (m hourAvail) Resolve(k provision.Key) (string, int64, bool) {
	id, ok := m[k]
	return id, time.Hour.Milliseconds(), ok
}
func (m hourAvail) ResolveEpisodes(provision.Key) schedule.EpisodeResolution {
	return schedule.EpisodeResolution{}
}

type fixedFades map[string][]schedule.BreakCandidate

func (f fixedFades) NaturalBreaks(id string) []schedule.BreakCandidate { return f[id] }

// Mid-roll is on by default for an internal channel with filler: reconcile splits a measured
// film at its fades. The per-channel switch and a Tunarr backend both keep it whole.
func TestReconcile_MidRollSplitsAtMeasuredFadesUnlessSwitchedOff(t *testing.T) {
	off := false
	for _, tc := range []struct {
		name      string
		backend   string
		midRoll   *bool
		wantParts int
	}{
		{name: "internal, policy absent (default on)", backend: schedule.PlayoutBackendInternal, wantParts: 4},
		{name: "internal, switched off", backend: schedule.PlayoutBackendInternal, midRoll: &off, wantParts: 0},
		{name: "tunarr never cuts inside a programme", backend: schedule.PlayoutBackendTunarr, wantParts: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newStore(t)
			var asked int
			fades := fixedFades{"lib-1": {{AtMs: 15 * 60_000, Confidence: 1}, {AtMs: 30 * 60_000, Confidence: 1}, {AtMs: 44 * 60_000, Confidence: 1}}}
			e := channels.New(st, tunarrtest.NewTunarr(), hourAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}, nil, channels.Config{
				ReconcileTTL: 10 * time.Minute, BreaksPerHour: 4,
				ResolvePlayoutBackendContext: func(context.Context) (string, error) { return tc.backend, nil },
				NaturalBreaks: func(context.Context) schedule.NaturalBreakSource {
					asked++
					return fades
				},
			}, func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, testkit.Logger()).
				WithPods(&fakePods{ids: []string{"clip"}, duration: 30_000})
			ch := store.Channel{Lineup: []schedule.LineupEntry{entry("movie:tmdb:1", "Film"), entry("movie:tmdb:2", "Other")}}
			ch.ID, ch.Name, ch.Number, ch.Strategy, ch.Status = "mid", "Mid", 7, schedule.Sequential, schedule.StatusBuilding
			ch.Policy.MidRoll = tc.midRoll
			if _, err := st.SaveChannel(context.Background(), ch); err != nil {
				t.Fatal(err)
			}
			if err := e.Reconcile(context.Background(), "mid"); err != nil {
				t.Fatal(err)
			}
			got, err := st.GetChannel(context.Background(), "mid")
			if err != nil {
				t.Fatal(err)
			}
			parts, midBreaks := 0, 0
			for _, s := range got.Desired {
				if s.Segment > 0 {
					parts++
				}
				if s.MidRoll {
					midBreaks++
				}
			}
			if parts != tc.wantParts || (parts > 0 && midBreaks != parts-1) {
				t.Fatalf("parts = %d, mid-roll breaks = %d, want %d parts; desired %+v", parts, midBreaks, tc.wantParts, got.Desired)
			}
			if tc.wantParts == 0 && asked != 0 {
				t.Fatalf("candidate source consulted %d times for a channel that does not break mid-programme", asked)
			}
		})
	}
}

type swappableFades struct{ f fixedFades }

func (s *swappableFades) NaturalBreaks(id string) []schedule.BreakCandidate {
	return s.f.NaturalBreaks(id)
}

// Fades measured while a programme is on air never re-cut it: the next reconcile keeps it as it
// was accepted and splits only programmes beyond the freeze horizon.
func TestReconcile_MidRollNeverReSplitsAProgrammeOnAir(t *testing.T) {
	st := newStore(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	source := &swappableFades{} // nothing measured yet
	e := channels.New(st, nil, hourAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}, nil, channels.Config{
		ReconcileTTL: 10 * time.Minute, BreaksPerHour: 4,
		ResolvePlayoutBackendContext: func(context.Context) (string, error) { return schedule.PlayoutBackendInternal, nil },
		NaturalBreaks:                func(context.Context) schedule.NaturalBreakSource { return source },
	}, func() time.Time { return now }, testkit.Logger()).WithPods(&fakePods{ids: []string{"clip"}, duration: 30_000})
	seedChannel(t, st, "on-air", 8, entry("movie:tmdb:1", "First"), entry("movie:tmdb:2", "Second"))
	if err := e.Reconcile(context.Background(), "on-air"); err != nil {
		t.Fatal(err)
	}

	// Ten minutes in, both films are measured. The first is on air; the second starts after the
	// first ends (60 min + a break), beyond the 30 min freeze horizon.
	now = now.Add(10 * time.Minute)
	fade := []schedule.BreakCandidate{{AtMs: 15 * 60_000, Confidence: 1}, {AtMs: 30 * 60_000, Confidence: 1}}
	source.f = fixedFades{"lib-1": fade, "lib-2": fade}
	if err := e.Reconcile(context.Background(), "on-air"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetChannel(context.Background(), "on-air")
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]int{}
	for _, s := range got.Desired {
		if s.Segment > 0 {
			parts[s.LibraryItemID]++
		}
	}
	if parts["lib-1"] != 0 || parts["lib-2"] == 0 {
		t.Fatalf("parts = %v, want the on-air film whole and the later film split; desired %+v", parts, got.Desired)
	}
}
