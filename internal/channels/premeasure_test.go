package channels_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/tunarr/tunarrtest"
	"github.com/loomarr/loomarr/internal/testkit"
)

// Reconcile hands every library item an internal channel airs in the next 48 h to the measure
// worker (#1518), whether or not the programme is long enough for mid-roll, so a first airing
// starts with its keyframe index. Beyond the horizon nothing is asked for (never the whole
// library), and a Tunarr channel, which never reads Loomarr's measurements, asks for none.
func TestReconcile_PremeasuresEveryProgrammeInTheNext48Hours(t *testing.T) {
	const titles = 60 // one hour each: well past the horizon
	avail := hourAvail{}
	var lineup []schedule.LineupEntry
	for i := 1; i <= titles; i++ {
		key := provision.Key(fmt.Sprintf("movie:tmdb:%d", 90000000+i))
		avail[key] = fmt.Sprintf("lib-%d", i)
		lineup = append(lineup, entry(string(key), fmt.Sprintf("Film %d", i)))
	}
	for _, tc := range []struct {
		backend string
		want    bool
	}{
		{backend: schedule.PlayoutBackendInternal, want: true},
		{backend: schedule.PlayoutBackendTunarr, want: false},
	} {
		t.Run(tc.backend, func(t *testing.T) {
			st := newStore(t)
			var asked []string
			e := channels.New(st, tunarrtest.NewTunarr(), avail, nil, channels.Config{
				ReconcileTTL:                 10 * time.Minute,
				ResolvePlayoutBackendContext: func(context.Context) (string, error) { return tc.backend, nil },
				Premeasure: func(_ context.Context, ids []string) {
					asked = append(asked, ids...)
				},
			}, func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, testkit.Logger())
			ch := channelstore.Channel{Lineup: lineup}
			ch.ID, ch.Name, ch.Number, ch.Strategy, ch.Status = "pre", "Pre", 8, schedule.Sequential, schedule.StatusBuilding
			if _, err := st.SaveChannel(context.Background(), ch); err != nil {
				t.Fatal(err)
			}
			if err := e.Reconcile(context.Background(), "pre"); err != nil {
				t.Fatal(err)
			}
			if !tc.want {
				if len(asked) != 0 {
					t.Fatalf("a Tunarr channel asked to measure %v", asked)
				}
				return
			}
			if !slices.Contains(asked, "lib-1") || !slices.Contains(asked, "lib-48") {
				t.Fatalf("asked %v; want every item airing in the next 48 h, first to 48th", asked)
			}
			if slices.Contains(asked, "lib-50") || slices.Contains(asked, fmt.Sprintf("lib-%d", titles)) {
				t.Fatalf("asked %v; want nothing scheduled past the 48 h horizon", asked)
			}
			seen := map[string]bool{}
			for _, id := range asked {
				if seen[id] {
					t.Fatalf("asked %v; %s asked twice in one pass", asked, id)
				}
				seen[id] = true
			}
		})
	}
}
