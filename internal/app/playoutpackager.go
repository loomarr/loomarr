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

func (s packagerSource) ItemAt(ctx context.Context, channelID string, at time.Time) (playout.PackagerItem, error) {
	t0 := time.Now()
	airing, streamURL, err := s.res.AiringAt(ctx, channelID, at)
	if err != nil {
		return playout.PackagerItem{}, err
	}
	item := playout.PackagerItem{Label: string(airing.Kind) + ":" + airing.Identity, Remaining: airing.Remaining}
	if !airing.Playable() || streamURL == "" {
		return item, nil // a card slot: the packager slates it
	}
	item.Input, item.Seek = streamURL, airing.Offset
	t1 := time.Now()
	item.AudioTrack = s.res.AudioTrackFor(ctx, channelID, airing.LibraryItemID, streamURL)
	t2 := time.Now()
	_, item.Format = s.res.PlanFor(ctx, streamURL, playout.PlanBaseline)
	if s.log != nil {
		// The tune-in (G2) split's resolution half; the packager logs the encoder half.
		s.log.Info("packager: item resolved", "channel", channelID, "item", item.Label,
			"airing_ms", t1.Sub(t0).Milliseconds(), "audio_track_ms", t2.Sub(t1).Milliseconds(),
			"stream_facts_ms", time.Since(t2).Milliseconds())
	}
	if s.targetLUFS != nil {
		var note string
		if item.GainDB, note = playout.FillerGain(airing, s.targetLUFS()); note != "" && s.log != nil {
			s.log.Info("packager: "+note+" — airing filler at 0 dB", "channel", channelID, "title", airing.Title)
		}
	}
	return item, nil
}

// Output is the host and the output profile a channel's format encodes with at the lease's output
// ladder rung (#1520). The packager asks only for formats it serves; an unknown class gets the
// baseline, logged.
func (s packagerSource) Output(ctx context.Context, channelID string, class playout.FormatClass, rung int) (playout.HostProfile, playout.OutputProfile) {
	profile := s.res.Profile(ctx, rung)
	var gpu playout.GPUFilters
	if s.gpu != nil {
		gpu = s.gpu()
	}
	out, ok := playout.FormatOutput(class, profile)
	if !ok {
		if s.log != nil {
			s.log.Warn("packager: unknown output format; encoding the baseline", "channel", channelID, "format", string(class))
		}
		out, _ = playout.FormatOutput(playout.FormatBaseline, profile)
	}
	return playout.HostFor(profile.Encoder, s.tonemap != nil && s.tonemap(), gpu), out
}
