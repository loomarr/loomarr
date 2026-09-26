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
			e := channels.New(st, testkit.NewTunarr(), hourAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}, nil, channels.Config{
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
