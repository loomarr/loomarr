package app

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/prepared"
)

func TestSyntheticResolversSelectDistinctStableProgrammeVariants(t *testing.T) {
	epoch := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	schedule := syntheticProgrammeSchedule{epoch: epoch, duration: 6 * time.Second}
	specs := [2]prepared.Specification{
		syntheticPreparedSpecification("channel-a", 0, prepared.RenditionContract{}),
		syntheticPreparedSpecification("channel-a", 1, prepared.RenditionContract{}),
	}
	for _, test := range []struct {
		name        string
		now         time.Time
		fingerprint string
		source      string
	}{
		{name: "first programme", now: epoch.Add(time.Second), fingerprint: specs[0].SourceFingerprint, source: "source-0.mp4"},
		{name: "successive programme", now: epoch.Add(7 * time.Second), fingerprint: specs[1].SourceFingerprint, source: "source-1.mp4"},
		{name: "same programme remains stable", now: epoch.Add(11 * time.Second), fingerprint: specs[1].SourceFingerprint, source: "source-1.mp4"},
	} {
		t.Run(test.name, func(t *testing.T) {
			preparedResolver := syntheticPreparedResolver{specs: map[string][2]prepared.Specification{"channel-a": specs}, schedule: schedule, now: func() time.Time { return test.now }}
			window, ok, err := preparedResolver.ResolvePrepared(context.Background(), playout.TuneRequest{ChannelID: "channel-a"}, time.Time{})
			if err != nil || !ok {
				t.Fatalf("ResolvePrepared() ok=%t err=%v", ok, err)
			}
			liveResolver := syntheticLiveResolver{sources: map[string][2]string{"channel-a": {"source-0.mp4", "source-1.mp4"}}, schedule: schedule, now: func() time.Time { return test.now }}
			live, source, err := liveResolver.AiringNow(context.Background(), "channel-a")
			if err != nil {
				t.Fatalf("AiringNow() error = %v", err)
			}
			if window.Current.Specification.SourceFingerprint != test.fingerprint || source != test.source || live.Identity != window.Current.Identity.ContentID {
				t.Fatalf("prepared=%q source=%q live=%q, want fingerprint=%q source=%q matching identities", window.Current.Specification.SourceFingerprint, source, live.Identity, test.fingerprint, test.source)
			}
			if playhead := live.StartedAt.Add(live.Offset); !playhead.Equal(test.now) {
				t.Errorf("live playhead = %s, want current wall clock %s", playhead, test.now)
			}
			if live.Offset != window.Current.Offset {
				t.Errorf("live offset = %s, prepared offset = %s", live.Offset, window.Current.Offset)
			}
			// The HTTP programme adapter publishes this end coordinate. Both delivery
			// paths must report the same authoritative boundary after a mid-Airing join.
			if end := live.StartedAt.Add(live.Offset).Add(live.Remaining); !end.Equal(window.Current.Identity.EndsAt) {
				t.Errorf("live end = %s, prepared end = %s", end, window.Current.Identity.EndsAt)
			}
		})
	}
}
