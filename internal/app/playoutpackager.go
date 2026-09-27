package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
)

// packagerSource is the channel packager's view of the schedule and the host (#1512 phase 2). It
// resolves an item (airing, stream URL, audio track, stream facts, and a programme's watermark)
// straight from the playout resolver.
type packagerSource struct {
	res     *playoutResolver
	tonemap func() bool
	gpu     func() playout.GPUFilters
	// targetLUFS reads filler.target_lufs live, so a changed target applies at the next clip.
	targetLUFS func() string
	// watermark is the channel's bug for an output (channelWatermarks.For); nil draws none.
	watermark func(ctx context.Context, channelID string, enc playout.Encoder, width, height int) *playout.Watermark
	log       *slog.Logger
}

// watermarkFor is an item's bug resolver: a library programme's only, never a break's commercial, a
// filler clip (Source), a bumper or an ID.
func (s packagerSource) watermarkFor(channelID string, airing playout.Airing) playout.WatermarkFor {
	if s.watermark == nil || airing.Kind != schedule.SlotProgram || airing.Source != "" {
		return nil
	}
	return func(ctx context.Context, enc playout.Encoder, width, height int) *playout.Watermark {
		return s.watermark(ctx, channelID, enc, width, height)
	}
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
	item.Watermark = s.watermarkFor(channelID, airing)
	t1 := time.Now()
	item.AudioTrack = s.res.AudioTrackFor(ctx, channelID, airing.LibraryItemID, streamURL)
	t2 := time.Now()
	_, item.Format = s.res.PlanFor(ctx, streamURL, playout.PlanBaseline)
	t3 := time.Now()
	// A tune-in seeks from here rather than decoding up to a source GOP to reach the offset (#1595).
	item.Keyframe, item.KeyframeIndexed = s.res.indexedKeyframe(ctx, streamURL, airing.Offset)
	if s.log != nil {
		// The tune-in (G2) split's resolution half; the packager logs the encoder half.
		s.log.Info("packager: item resolved", "channel", channelID, "item", item.Label,
			"airing_ms", t1.Sub(t0).Milliseconds(), "audio_track_ms", t2.Sub(t1).Milliseconds(),
			"stream_facts_ms", t3.Sub(t2).Milliseconds(), "keyframe_ms", time.Since(t3).Milliseconds())
	}
	if s.targetLUFS != nil {
		var note string
		if item.GainDB, note = playout.FillerGain(airing, s.targetLUFS()); note != "" && s.log != nil {
			s.log.Info("packager: "+note+" — airing filler at 0 dB", "channel", channelID, "title", airing.Title)
		}
	}
	return item, nil
}

// Premium is the premium format the channel's lineup warrants, as this host airs it (#1512 G10):
// GET /v1/channels/{id}/formats' derivation, on the host the packager encodes with. A lineup that
// cannot be read airs none.
func (s packagerSource) Premium(ctx context.Context, channelID string) playout.FormatClass {
	lineup, err := s.res.LineupFormats(ctx, channelID)
	if err != nil {
		if s.log != nil {
			s.log.Warn("packager: lineup formats unavailable; offering the baseline only", "channel", channelID, "err", err)
		}
		return ""
	}
	host, _ := s.Output(ctx, channelID, playout.FormatBaseline, 0)
	aired, _ := playout.DeriveChannelFormats(lineup).OnHost(host)
	return aired.Premium
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
	// The live playout.tone_curve, the curve the capacity probe measured HDR with.
	out.ToneCurve = s.res.ToneCurve()
	return playout.HostFor(profile.Encoder, s.tonemap != nil && s.tonemap(), gpu), out
}
