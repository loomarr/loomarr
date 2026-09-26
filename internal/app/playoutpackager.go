package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// packagerSource is the channel packager's view of the schedule and the host (#1512 phase 2). It
// resolves an item the way the /v1/playout/program handler does (airing, stream URL, audio track,
// stream facts) without the loopback HTTP hop.
type packagerSource struct {
	res     *playoutResolver
	tonemap func() bool
	gpu     func() playout.GPUFilters
	// targetLUFS reads filler.target_lufs live, so a changed target applies at the next clip.
	targetLUFS func() string
	log        *slog.Logger
}

func (s packagerSource) ItemAt(ctx context.Context, channelID string, plan playout.EncodePlan, at time.Time) (playout.PackagerItem, error) {
	airing, streamURL, err := s.res.AiringAt(ctx, channelID, at)
	if err != nil {
		return playout.PackagerItem{}, err
	}
	item := playout.PackagerItem{Label: string(airing.Kind) + ":" + airing.Identity, Remaining: airing.Remaining}
	if !airing.Playable() || streamURL == "" {
		return item, nil // a card slot: the packager slates it
	}
	item.Input, item.Seek = streamURL, airing.Offset
	item.AudioTrack = s.res.AudioTrackFor(ctx, channelID, airing.LibraryItemID, streamURL)
	_, item.Format = s.res.PlanFor(ctx, streamURL, plan)
	if s.targetLUFS != nil {
		var note string
		if item.GainDB, note = playout.FillerGain(airing, s.targetLUFS()); note != "" && s.log != nil {
			s.log.Info("packager: "+note+" — airing filler at 0 dB", "channel", channelID, "title", airing.Title)
		}
	}
	return item, nil
}

func (s packagerSource) Output(ctx context.Context, _ string, _ playout.EncodePlan) (playout.HostProfile, playout.OutputProfile) {
	profile := s.res.Profile(ctx)
	var gpu playout.GPUFilters
	if s.gpu != nil {
		gpu = s.gpu()
	}
	return playout.HostFor(profile.Encoder, s.tonemap != nil && s.tonemap(), gpu), playout.ChannelOutput(profile)
}
