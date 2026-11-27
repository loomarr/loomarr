package operator

import (
	"context"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

type standingStore struct{ chans []channelstore.Channel }

func (s standingStore) GetChannel(context.Context, string) (channelstore.Channel, error) {
	return channelstore.Channel{}, nil
}
func (s standingStore) ListChannels(context.Context) ([]channelstore.Channel, error) {
	return s.chans, nil
}
func (standingStore) GetClip(context.Context, string) (clipstore.Clip, error) {
	return clipstore.Clip{}, nil
}
func (standingStore) CountClips(context.Context, clipstore.ClipFilter) (int, error) { return 0, nil }

type standingCatalog []pod.Clip

func (c standingCatalog) AllClips(context.Context) ([]pod.Clip, error) { return c, nil }

func standingChannel(id string, status schedule.ChannelStatus, excluded ...string) channelstore.Channel {
	return channelstore.Channel{Channel: schedule.Channel{
		ID: id, Status: status,
	}, Policy: schedule.ChannelPolicy{Filler: &schedule.FillerSelection{Excluded: excluded}}}
}

// A paused channel resumes, so the clips only it can air are not idle: pausing a channel is not
// deleting its filler. A detached channel is no longer Loomarr's, so clips only it can air are.
// The pool strip, which reports on breaks that are airing, still leaves the paused channel out.
func TestStandingCountsAPausedChannelButNotADetachedOne(t *testing.T) {
	ad := func(hash string) pod.Clip {
		return pod.Clip{Hash: hash, Path: hash + ".mp4", Role: pod.Commercial, Era: 1994, Audience: pod.General, DurationMs: 30_000}
	}
	// Each of the first two excludes what the other channels may not use: "paused only" is
	// something only the paused channel can air, "detached only" only the detached one.
	store := standingStore{chans: []channelstore.Channel{
		standingChannel("live", schedule.StatusLive, "paused only", "detached only"),
		standingChannel("paused", schedule.StatusPaused, "detached only"),
		standingChannel("detached", schedule.StatusDetached, "paused only"),
	}}
	adapter := pod.NewAdapter(standingCatalog{ad("for everyone"), ad("paused only"), ad("detached only")}, nil,
		func() pod.Policy { return pod.Policy{PodMax: 4, BreakDurationMs: 120_000, Cooldown: time.Hour} }, slog.New(slog.DiscardHandler))
	preview := NewPodPreview(store, adapter)

	got, err := preview.Standing(context.Background(), nil, time.Hour, time.Date(2026, 11, 14, 20, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Idle, []string{"detached only"}) || len(got.Surplus) != 0 {
		t.Fatalf("standing = %+v, want only the clip no managed channel can air idle", got)
	}

	pool, err := preview.liveChannels(context.Background())
	if err != nil || len(pool) != 1 || pool[0].ChannelID != "live" {
		t.Fatalf("live channels = %+v, %v; want the live channel only", pool, err)
	}
}
