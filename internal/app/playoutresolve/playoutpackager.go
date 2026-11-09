package playoutresolve

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
)

// PackagerSource is the channel packager's view of the schedule and the host (#1512). It
// resolves an item (airing, stream URL, audio track, stream facts, and a programme's watermark)
// straight from the playout resolver.
type PackagerSource struct {
	Res     *Resolver
	Tonemap func() bool
	GPU     func() encode.GPUFilters
	// TargetLUFS reads filler.target_lufs live, so a changed target applies at the next clip.
	TargetLUFS func() string
	// Watermark is the channel's bug for an output at an airing's start (channelbug.Service.For);
	// nil draws none.
	Watermark func(ctx context.Context, channelID string, enc encode.Encoder, width, height int, at time.Time) *playout.Watermark
	Log       *slog.Logger
}

// WatermarkFor is an item's bug resolver: a library programme's only, never a break's commercial, a
// filler clip (Source), a bumper or an ID.
func (s PackagerSource) WatermarkFor(channelID string, airing playout.Airing) playout.WatermarkFor {
	if s.Watermark == nil || airing.Kind != schedule.SlotProgram || airing.Source != "" {
		return nil
	}
	return func(ctx context.Context, enc encode.Encoder, width, height int) *playout.Watermark {
		return s.Watermark(ctx, channelID, enc, width, height, airing.StartedAt)
	}
}

func (s PackagerSource) ItemAt(ctx context.Context, channelID string, at time.Time) (playout.PackagerItem, error) {
	t0 := time.Now()
	airing, streamURL, err := s.Res.AiringAt(ctx, channelID, at)
	if err != nil {
		return playout.PackagerItem{}, err
	}
	item := playout.PackagerItem{Label: string(airing.Kind) + ":" + airing.Identity, Remaining: airing.Remaining}
	if !airing.Playable() || streamURL == "" {
		return item, nil // a card slot: the packager slates it
	}
	item.Input, item.Seek = streamURL, airing.Offset
	item.Watermark = s.WatermarkFor(channelID, airing)
	item.RecordAiringServed = func(ctx context.Context) { s.Res.RecordAiringServed(ctx, channelID, airing, at) }
	t1 := time.Now()
	item.AudioTrack = s.Res.AudioTrackFor(ctx, channelID, airing.LibraryItemID, streamURL)
	t2 := time.Now()
	item.Format = s.Res.FormatOf(ctx, airing.LibraryItemID, streamURL)
	t3 := time.Now()
	// A tune-in seeks from here rather than decoding up to a source GOP to reach the offset (#1595).
	item.Keyframe, item.KeyframeIndexed = s.Res.indexedKeyframe(ctx, streamURL, airing.Offset)
	if s.Log != nil {
		// The tune-in (G2) split's resolution half; the packager logs the encoder half.
		s.Log.Info("packager: item resolved", "channel", channelID, "item", item.Label,
			"airing_ms", t1.Sub(t0).Milliseconds(), "audio_track_ms", t2.Sub(t1).Milliseconds(),
			"stream_facts_ms", t3.Sub(t2).Milliseconds(), "keyframe_ms", time.Since(t3).Milliseconds())
	}
	if s.TargetLUFS != nil {
		var note string
		if item.GainDB, note = playout.FillerGain(airing, s.TargetLUFS()); note != "" && s.Log != nil {
			s.Log.Info("packager: "+note+" — airing filler at 0 dB", "channel", channelID, "title", airing.Title)
		}
	}
	return item, nil
}

// Premium is the premium format the channel's lineup warrants, as this host airs it (#1512 G10):
// GET /v1/channels/{id}/formats' derivation, on the host the packager encodes with. A lineup that
// cannot be read airs none.
func (s PackagerSource) Premium(ctx context.Context, channelID string) playout.FormatClass {
	lineup, err := s.Res.LineupFormats(ctx, channelID)
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("packager: lineup formats unavailable; offering the baseline only", "channel", channelID, "err", err)
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
func (s PackagerSource) Output(ctx context.Context, channelID string, class playout.FormatClass, rung int) (encode.HostProfile, playout.OutputProfile) {
	profile := s.Res.Profile(ctx, rung)
	var gpu encode.GPUFilters
	if s.GPU != nil {
		gpu = s.GPU()
	}
	out, ok := playout.FormatOutput(class, profile)
	if !ok {
		if s.Log != nil {
			s.Log.Warn("packager: unknown output format; encoding the baseline", "channel", channelID, "format", string(class))
		}
		out, _ = playout.FormatOutput(playout.FormatBaseline, profile)
	}
	// The live playout.tone_curve, the curve the capacity probe measured HDR with.
	out.ToneCurve = s.Res.ToneCurve()
	return encode.HostFor(profile.Encoder, s.Tonemap != nil && s.Tonemap(), gpu), out
}
