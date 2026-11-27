package playoutresolve

import (
	"context"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout"
)

// ComputeChannelCodec derives the channel's uniform BROADCAST CODEC (§9.1 V50) from its library
// content: it samples the channel's program slots, probes each file's video codec, and takes the
// MAJORITY (Q2). The result is what the timeline normalizes to — the show that already matches
// copies, everything else (minority-codec titles, all filler) transcodes to match — which is what
// keeps the stream single-codec and therefore HEVC-fMP4-legal.
//
// Called at CURATION (after a bind writes the lineup), not on the play hot path: the codec is
// stored (Q1) so the session start reads a column, not a probe. It reuses the SAME resolve+probe
// path AiringNow uses (CyclePreview → itemInput → probeFormat), so the codec we store is
// measured from the very files we broadcast — it cannot drift from what actually plays.
//
// Returns BroadcastCodecH264 as the safe answer whenever it cannot measure (no probe wired, no
// resolvable files, an empty/all-pending lineup): h264/TS plays on every client, so an
// un-measurable channel stays maximally compatible rather than guessing HEVC. Ties break to h264
// for the same reason — a 50/50 channel is not clearly HEVC-dominant.
//
// If r.codecs is wired the measured codec is persisted; the measured value is returned either way.
func (r *Resolver) ComputeChannelCodec(ctx context.Context, channelID string) (string, error) {
	if r.codecs == nil {
		return r.measureChannelCodec(ctx, channelID), nil
	}
	if r.channels == nil {
		return channelstore.BroadcastCodecH264, errors.New("persist broadcast codec: channel reader is not configured")
	}

	// Measuring spans cycle construction, library resolution, and several probes. A lineup can
	// change during that window, so the derived codec is only valid when the channel revision is
	// unchanged on both sides of the measurement. The targeted write uses that same revision and
	// advances it, preventing either a stale probe or a full-row writer from overwriting the other.
	const maxCodecWriteAttempts = 4
	codec := channelstore.BroadcastCodecH264
	for attempt := 0; attempt < maxCodecWriteAttempts; attempt++ {
		before, err := r.channels.GetChannel(ctx, channelID)
		if err != nil {
			return codec, fmt.Errorf("read channel %s before codec measurement: %w", channelID, err)
		}
		codec = r.measureChannelCodec(ctx, channelID)
		after, err := r.channels.GetChannel(ctx, channelID)
		if err != nil {
			return codec, fmt.Errorf("read channel %s after codec measurement: %w", channelID, err)
		}
		if before.Revision != after.Revision {
			continue
		}
		if _, err := r.codecs.SetChannelBroadcastCodec(ctx, channelID, after.Revision, codec); err == nil {
			return codec, nil
		} else if !errors.Is(err, channelstore.ErrChannelStale) {
			return codec, fmt.Errorf("persist broadcast codec for %s: %w", channelID, err)
		}
		if err := ctx.Err(); err != nil {
			return codec, err
		}
	}
	return codec, fmt.Errorf("persist broadcast codec for %s: channel kept changing", channelID)
}

// ChannelCodec reads the channel's STORED broadcast codec (§9.1 V50) — the hot-path read the play
// URL and program routing use to decide the served container + the transcode target. It is a plain
// column read (set at curation by ComputeChannelCodec / backfilled at boot), NOT a probe: the whole
// point of storing it (Q1) is that a session start costs a row read, not N ffprobes.
//
// Defaults to BroadcastCodecH264 on any miss (no channel reader wired, channel not found, or an
// empty column on an un-backfilled row) — h264/TS is the maximally-compatible fallback, so a
// lookup failure degrades a channel to "plays everywhere", never to a black screen.
func (r *Resolver) ChannelCodec(ctx context.Context, channelID string) string {
	if r.channels == nil {
		return channelstore.BroadcastCodecH264
	}
	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		if r.log != nil {
			r.log.Debug("codec: channel read failed; defaulting h264", "channel", channelID, "err", err)
		}
		return channelstore.BroadcastCodecH264
	}
	if ch.BroadcastCodec == "" {
		return channelstore.BroadcastCodecH264
	}
	return ch.BroadcastCodec
}

// LineupFormats returns the stream facts of each distinct programme in the channel's cycle, the
// input to playout.DeriveChannelFormats (#1512 G10). The facts are the scanner's read of each
// item's playing file, never probed. An item the Library has not read is the zero MediaFormat,
// which the derivation ignores.
func (r *Resolver) LineupFormats(ctx context.Context, channelID string) ([]playout.MediaFormat, error) {
	_, slots, _, _, err := r.engine.CyclePreview(ctx, channelID, r.now())
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var formats []playout.MediaFormat
	for _, sl := range slots {
		// Filler and pending acquisitions are SDR and file-less; only landed programmes can warrant 4K/HDR.
		if sl.Kind != schedule.SlotProgram || sl.LibraryItemID == "" || seen[sl.LibraryItemID] {
			continue
		}
		seen[sl.LibraryItemID] = true
		format, _ := r.libraryFormat(ctx, sl.LibraryItemID)
		formats = append(formats, format)
	}
	return formats, nil
}

// maxCodecProbes bounds how many program files ComputeChannelCodec probes to decide the majority.
//
// Curation is a human-triggered action, not a hot loop, but a household lineup can still be scores
// of titles and each probe is an ffprobe exec (~tens–hundreds of ms). Sampling the first N program
// slots is enough to establish the majority codec reliably — a channel's library is overwhelmingly
// one codec in practice, and a genuinely mixed one is decided by the same N-sample majority the
// full scan would reach. Past N, the tail would only confirm the lead. Bounds the approve latency.
const maxCodecProbes = 12

// measureChannelCodec is the pure-ish measurement half of ComputeChannelCodec: sample → probe →
// majority, no persistence. Split out so the storing wrapper stays trivial and the sampling policy
// (which slots, how many, tiebreak) lives in one place.
func (r *Resolver) measureChannelCodec(ctx context.Context, channelID string) string {
	if r.probeFormat == nil {
		return channelstore.BroadcastCodecH264 // cannot measure → safe default
	}
	_, slots, _, _, err := r.engine.CyclePreview(ctx, channelID, r.now())
	if err != nil {
		if r.log != nil {
			r.log.Debug("codec: cycle preview failed; defaulting h264", "channel", channelID, "err", err)
		}
		return channelstore.BroadcastCodecH264
	}

	var codecs []string
	probed := 0
	for _, sl := range slots {
		if probed >= maxCodecProbes {
			break
		}
		// Only real, landed PROGRAMS carry a file to probe. Pending acquisitions have no
		// LibraryItemID yet, and filler/flex are normalized TO the channel codec, not sources of it.
		if sl.Kind != schedule.SlotProgram || sl.LibraryItemID == "" {
			continue
		}
		input := r.itemInput(ctx, sl.LibraryItemID)
		if input == "" {
			continue
		}
		fmtInfo, perr := r.probeFormat(ctx, input)
		if perr != nil {
			if r.log != nil {
				r.log.Debug("codec: probe failed; skipping title", "channel", channelID, "item", sl.LibraryItemID, "err", perr)
			}
			continue
		}
		probed++
		codecs = append(codecs, fmtInfo.VideoCodec)
	}
	return majorityBroadcastCodec(codecs)
}

// majorityBroadcastCodec turns a sample of probed video-codec strings into the one broadcast codec
// the channel normalizes to (§9.1 V50 Q2: MAJORITY WINS). Everything non-HEVC counts as h264 —
// h264/mpeg2/vp9/… all normalize DOWN to h264 — so the vote is strictly HEVC vs. not-HEVC.
//
// An even split (including the empty sample: no probes, or an all-pending/filler lineup) breaks to
// h264, the maximally-compatible choice: h264/TS needs no fMP4 and no HEVC decoder, so a channel
// that isn't clearly HEVC-dominant stays playable everywhere. Pure + total, so the sampling policy
// above and this decision are testable in isolation.
func majorityBroadcastCodec(codecs []string) string {
	var hevc, other int
	for _, c := range codecs {
		if playout.IsHEVCCodec(c) {
			hevc++
		} else {
			other++
		}
	}
	if hevc > other {
		return channelstore.BroadcastCodecHEVC
	}
	return channelstore.BroadcastCodecH264
}
