package playout

import (
	"context"
	"math"
	"strings"
)

// The copy/transcode decision (§9.1 V47): what can we COPY versus what must we transcode?
//
// The premise of direct play is that most library files are already a codec the target can play
// (h264 is the near-universal case), so re-encoding is wasted CPU/GPU and adds latency. But the
// decision is PER STREAM, not per file — the video and audio are independent. The common real case
// is an h264 video with a surround audio track (EAC3/AC3) a browser cannot decode: the right answer
// is to COPY the video (the expensive part, instant, no GPU) and re-encode ONLY the audio (cheap).
//
// This is the industry-standard three-way every media server makes:
//   - direct play    — copy both (video + audio already compatible)
//   - direct stream   — copy video, transcode audio (the h264 + EAC3 → browser case)
//   - transcode       — re-encode video (the codec itself is incompatible: HEVC/MPEG-2 → browser)
//
// The MediaFormat it decides over is probed once from the actual media (probe.go), never the media
// server's metadata.

// MediaFormat is everything a probe tells us about a source that playout might use — the copy
// decision needs the codecs; the rest (dimensions, framerate, HDR, bit depth, duration, bitrate,
// container) is captured in the SAME probe so later features (framerate-aware transcode, HDR
// tone-mapping, quality display, subtitle burn-in) need no second ffprobe. Probe once, keep it all.
type MediaFormat struct {
	// Video.
	VideoCodec    string // e.g. "h264", "hevc", "mpeg2video" — lowercased; empty when no video
	Width         int
	Height        int
	FrameRate     float64 // frames/sec, from avg_frame_rate; 0 when unknown
	PixelFormat   string  // e.g. "yuv420p", "yuv420p10le" — the 10le suffix signals 10-bit
	ColorTransfer string  // e.g. "smpte2084" (HDR10/PQ), "arib-std-b67" (HLG); "" for SDR

	// Audio (the first/primary track — track SELECTION is a separate concern, see audio.go).
	AudioCodec    string // e.g. "aac", "eac3", "ac3" — lowercased; empty when no audio
	AudioChannels int    // 2 = stereo, 6 = 5.1, …; 0 when unknown

	// Container + overall.
	Container string  // format_name, e.g. "matroska,webm", "mov,mp4,…"; "" when unknown
	Duration  float64 // seconds; 0 when unknown
	Bitrate   int64   // overall bit/s; 0 when unknown
}

// IsHEVCCodec reports whether a codec-name string names HEVC/H.265. ffprobe emits "hevc" but
// some sources/containers label the same codec "h265", so both count — the single place that
// equivalence lives (used by the copy-plan bucketing and the §9.1 V50 channel-codec measurement).
func IsHEVCCodec(codec string) bool {
	c := strings.ToLower(codec)
	return c == "hevc" || c == "h265"
}

// TenBit reports whether the video is 10-bit (or deeper) — the `…10le`/`…12le` pixel-format
// suffix. Relevant because some encoders (e.g. h264_nvenc) only do 8-bit, so a 10-bit source
// forces a pixel-format conversion on transcode (a trap recorded in PROGRESS.md).
func (f MediaFormat) TenBit() bool {
	return strings.Contains(f.PixelFormat, "10le") || strings.Contains(f.PixelFormat, "12le")
}

// HDR reports whether the video carries an HDR transfer function (PQ/HLG) — SDR clients need
// tone-mapping on transcode. Captured now so the transcode path can act on it later.
func (f MediaFormat) HDR() bool {
	t := strings.ToLower(f.ColorTransfer)
	return t == "smpte2084" || t == "arib-std-b67"
}

// FormatProber reports a source's MediaFormat. An interface so callers are testable without exec,
// mirroring AudioProber / TrackProber. The concrete prober (FFprobeFormatNextTo) lives in probe.go.
type FormatProber func(ctx context.Context, input string) (MediaFormat, error)

// EncodePlan is the CANONICAL codec bucket a session is keyed on and encoded for (§9.1 V48). It is
// NOT the client's capability — that is DeviceProfile. Many profiles resolve() into few EncodePlans,
// so encoder fan-out is bounded by this small fixed set, never by the number of distinct devices.
//
// The order is widening capability, and it is load-bearing: resolve() picks the richest plan a
// profile fully satisfies, and String()/ParseEncodePlan round-trip the wire token. `baseline` is the
// safe default the whole design falls back to (a client that proves nothing gets h264/aac).
type EncodePlan int

const (
	// PlanBaseline is h264 video + AAC audio — what a plain browser `<video>`/MSE and Safari/iOS
	// decode with no hardware help. The safe default: any client that has not ADVERTISED more gets
	// this, so a client with no HEVC decoder never receives an undecodable stream.
	PlanBaseline EncodePlan = iota
	// PlanHEVC8 adds 8-bit HEVC video — a browser with a hardware HEVC decoder (hls.js ≥1.6
	// transmuxes HEVC-in-MPEG-TS for MSE) or a modest native app. Audio stays AAC (the browser
	// baseline); HEVC files carrying EAC3/AC3 surround still transcode that audio alone.
	PlanHEVC8
	// PlanHEVC10 adds 10-bit HEVC and surround passthrough (EAC3/AC3) — a capable native app
	// (AVPlayer/ExoPlayer) that decodes 10-bit and plays surround directly, so neither the video
	// nor the audio needs a transcode for typical HEVC content.
	PlanHEVC10
	// PlanFull is the broadest set — h264/hevc video + aac/ac3/eac3/mp3 audio. It is what a
	// media-server tuner (Emby/Jellyfin) ingests and re-transcodes downstream, and the plan the
	// tuner path (which sends no DeviceProfile) resolves to. Was the old `mediaserver` target.
	PlanFull
)

// String renders a plan as the wire token (the `?plan=` query on the playlist and program URLs) and
// the session-key component. Stable, lowercase, the inverse of ParseEncodePlan.
func (p EncodePlan) String() string {
	switch p {
	case PlanHEVC8:
		return "hevc8"
	case PlanHEVC10:
		return "hevc10"
	case PlanFull:
		return "full"
	default:
		return "baseline"
	}
}

// ParseEncodePlan maps a wire token back to an EncodePlan. Unknown or empty ⇒ PlanBaseline — the
// SAFE default (h264/aac): the copy plan that plays on any client, so an absent/garbage `?plan=`
// never grants a capability the client did not prove. A client widens EXPLICITLY, via a recognized
// token minted from its own DeviceProfile. (This is the inverse of the old ParseTarget, whose unknown
// default was the BROAD mediaserver set — retired in V48 precisely because "unknown ⇒ broad" is the
// wrong default for the browser/Watch path. The tuner path resolves to PlanFull explicitly, not by
// falling through here.)
func ParseEncodePlan(s string) EncodePlan {
	switch s {
	case "hevc8":
		return PlanHEVC8
	case "hevc10":
		return PlanHEVC10
	case "full":
		return PlanFull
	default:
		return PlanBaseline
	}
}

// copyVideo / copyAudio are the codecs each plan copies without a transcode. A plan copies a superset
// of the plan below it; 10-bit is gated separately (see PlanCopy) since it is a pixel-format axis, not
// a codec-name one.
var planCopyVideo = map[EncodePlan]map[string]bool{
	PlanBaseline: {"h264": true},
	PlanHEVC8:    {"h264": true, "hevc": true, "h265": true},
	PlanHEVC10:   {"h264": true, "hevc": true, "h265": true},
	PlanFull:     {"h264": true, "hevc": true, "h265": true},
}

var planCopyAudio = map[EncodePlan]map[string]bool{
	PlanBaseline: {"aac": true},
	PlanHEVC8:    {"aac": true},
	PlanHEVC10:   {"aac": true, "ac3": true, "eac3": true},
	PlanFull:     {"aac": true, "ac3": true, "eac3": true, "mp3": true},
}

// DeviceProfile is what a Watch CLIENT advertises it can direct-play (§9.1 V48) — the input to
// resolve(). Sent as a JSON body on POST /v1/channels/{id}/play-url. All fields are optional and the
// ZERO value is the safe baseline: an absent/empty profile resolves to PlanBaseline, so a client that
// proves nothing gets h264/aac. A browser fills it from MediaSource.isTypeSupported(…); a native app
// from its known decoder set.
type DeviceProfile struct {
	// Video / Audio are the codecs the client can DECODE, lowercased (e.g. "hevc", "eac3"). h264 and
	// aac are the implied floor — a profile need not list them, and resolve treats them as present.
	Video []string
	Audio []string
	// Video10Bit reports the client can decode 10-bit (or deeper) video. Gates PlanHEVC10's 10-bit
	// copy: without it, a 10-bit HEVC source transcodes even for an HEVC-capable client.
	Video10Bit bool
	// HDR reports the client can DISPLAY HDR (PQ/HLG). Drives the tone-map decision on transcode, not
	// the copy-codec bucket, so it does not multiply EncodePlans.
	HDR bool
	// MaxResolution is the tallest video height the client will direct-play (e.g. 1080). 0 = no cap.
	// Drives the rendition ladder, not the copy bucket — kept out of resolve() for the same reason.
	MaxResolution int
}

// has reports whether `codec` is in `list` (case-insensitive), treating `implied` as always present.
func has(list []string, codec, implied string) bool {
	if codec == implied {
		return true
	}
	for _, c := range list {
		if strings.EqualFold(strings.TrimSpace(c), codec) {
			return true
		}
	}
	return false
}

// resolve maps a DeviceProfile to the richest EncodePlan the profile FULLY satisfies, rounding DOWN
// when a profile sits between buckets (§9.1 V48). It is pure, total, and the ONE place bucketing
// lives. It never returns a plan that claims a capability the profile did not advertise — the
// black-frame guard: a client that did not prove HEVC (or 10-bit, or surround) is bucketed below it.
//
// It deliberately does NOT return PlanFull: `full` is the tuner set (mp3 + the broad audio), reached
// only by the tuner path resolving to it explicitly, not by a browser/native profile. The richest a
// client profile earns is PlanHEVC10.
func resolve(p DeviceProfile) EncodePlan {
	hevc := has(p.Video, "hevc", "h264") || has(p.Video, "h265", "h264")
	surround := has(p.Audio, "eac3", "aac") || has(p.Audio, "ac3", "aac")
	switch {
	case hevc && p.Video10Bit && surround:
		return PlanHEVC10
	case hevc:
		return PlanHEVC8
	default:
		return PlanBaseline
	}
}

// ResolvePlan is the exported entry point callers use to bucket a client's advertised capabilities
// into the session's EncodePlan (§9.1 V48). Thin wrapper over the pure resolve so the bucketing logic
// stays unexported and single-sourced. (Named ResolvePlan, not Resolve, because quality.go already
// owns Resolve for the quality-tier→Profile decision — a different resolution entirely.)
//
// ⚠ V50 (below) makes ServedPlan the entry point the play URL uses — it gates on the CHANNEL's codec,
// not the client's capabilities. ResolvePlan remains the pure profile-richness helper ServedPlan
// composes, and the one place the hevc8-vs-hevc10 (10-bit + surround) distinction lives.
func ResolvePlan(p DeviceProfile) EncodePlan { return resolve(p) }

// ServedPlan decides the EncodePlan a channel is served AS to a given client (§9.1 V50 — the
// content-driven model). Two independent axes decide it:
//
//   - channelCodec — the channel's UNIFORM broadcast codec (store.broadcast_codec), derived from its
//     library content. This is what the timeline normalizes to, and it is what makes the HEVC/fMP4
//     path legal (one codec end to end). An h264 channel is ALWAYS served h264/TS — no client can
//     promote it to HEVC, because the timeline isn't HEVC to begin with.
//   - the client DeviceProfile — a yes/no gate on whether this client can decode the channel's native
//     codec. It never PICKS the channel codec; it only decides copy-native vs down-convert.
//
// The result maps onto the existing EncodePlan container/copy machinery unchanged:
//   - h264 channel                    → PlanBaseline (h264/TS, copy the h264 show).
//   - hevc channel, client plays hevc → PlanHEVC8/10 (fMP4, copy the hevc show; 10-bit+surround
//     picks hevc10 via the profile-richness helper).
//   - hevc channel, client can't      → PlanBaseline (down-convert the whole channel to h264/TS).
//
// So WantsHEVCOutput()/the fMP4 container/the HEVC transcode target all key on the plan exactly as
// before — but the plan now expresses "how this channel is served", not "the richest bucket the
// client supports". PlanFull (tuner) is never returned here; the tuner path resolves it explicitly.
func ServedPlan(channelCodec string, p DeviceProfile) EncodePlan {
	if !IsHEVCCodec(channelCodec) {
		return PlanBaseline // h264 (or any non-hevc) channel: served h264/TS to everyone.
	}
	// HEVC channel. Serve it natively ONLY to a client that proved it can decode HEVC; otherwise
	// down-convert the whole channel to h264/TS for this client (its own session, keyed on the plan).
	clientHEVC := has(p.Video, "hevc", "h264") || has(p.Video, "h265", "h264")
	if !clientHEVC {
		return PlanBaseline // capable-channel, incapable-client → down-convert.
	}
	// Native HEVC copy. resolve() picks hevc8 vs hevc10 from the profile's 10-bit + surround richness
	// — but it can also fall to PlanBaseline if the profile is threadbare, which for an HEVC channel a
	// client already proved it can take, so floor it at hevc8 (never serve an HEVC channel as TS to a
	// client that CAN play HEVC — that would needlessly transcode).
	if plan := resolve(p); plan == PlanHEVC8 || plan == PlanHEVC10 {
		return plan
	}
	return PlanHEVC8
}

// WantsHEVCOutput reports whether this plan's stream must be HEVC-coded end to end (§9.1 V49). The
// HEVC plans (hevc8/hevc10) are served over fMP4, which cannot mix codecs mid-stream, so any program
// they transcode must output HEVC, not the h264 default. Baseline (h264/TS) and full (tuner) do not.
func (p EncodePlan) WantsHEVCOutput() bool {
	return p == PlanHEVC8 || p == PlanHEVC10
}

// EstimatedCost is a plan's admission cost BEFORE any program has reported (§9.1 V49 admission).
// It is the conservative guess used at session start, corrected to the real cost once the first
// program reports whether it actually transcoded (see Manager.ReportProgram):
//
//   - PlanBaseline MIGHT transcode (an HEVC/incompatible channel to an h264-only client) → cost 1.
//   - The HEVC/full plans COPY the video (that is their whole point) → cost 0.
//
// Over-counting baseline on an h264 channel (guessing 1 when it will copy) is SAFE — it never
// over-admits — and self-corrects to 0 on the first program report. The video transcode is the only
// thing that consumes the GPU budget; audio transcode (a cheap AAC encode) is not counted.
func (p EncodePlan) EstimatedCost() int {
	if p == PlanBaseline {
		return 1
	}
	return 0
}

// CopyPlan is the per-stream copy/transcode decision for one source against one EncodePlan. It maps
// directly onto ffmpeg codec flags: CopyVideo → `-c:v copy` else a video encoder; CopyAudio →
// `-c:a copy` else `-c:a aac`.
type CopyPlan struct {
	CopyVideo bool
	CopyAudio bool
}

// DirectPlay reports whether NOTHING needs transcoding — both streams copy. The fastest, cheapest
// path (no encode at all).
func (p CopyPlan) DirectPlay() bool { return p.CopyVideo && p.CopyAudio }

// Cost is the REAL admission cost of a program once its copy plan is known (§9.1 V49): 1 when the
// VIDEO is transcoded (the GPU-consuming case), else 0. Audio transcode (a cheap AAC encode) does
// not count — the budget bounds concurrent VIDEO encodes, which is what saturates the box.
func (p CopyPlan) Cost() int {
	if p.CopyVideo {
		return 0
	}
	return 1
}

// PlanCopy decides, per stream, what can be copied for `plan` — the core decision, pure and testable
// without ffmpeg.
//
//   - Video copies when the plan copies its codec AND the bit depth is allowed. Only PlanHEVC10 (and
//     PlanFull) copy 10-bit; a 10-bit HEVC source to PlanHEVC8 TRANSCODES, because that client proved
//     8-bit HEVC only — copying 10-bit to it is the same black frame as copying HEVC to baseline.
//   - Audio copies when the plan copies its codec, OR when there is no audio. Otherwise the audio
//     alone transcodes (to AAC) — the video is untouched. This is what makes an h264 + EAC3 file
//     cost only a cheap audio encode for a baseline client, not a full video re-encode.
func PlanCopy(f MediaFormat, plan EncodePlan) CopyPlan {
	v := strings.ToLower(strings.TrimSpace(f.VideoCodec))
	a := strings.ToLower(strings.TrimSpace(f.AudioCodec))
	copyVideo := planCopyVideo[plan][v]
	// 10-bit is a pixel-format axis, not a codec-name one: a plan may copy the CODEC (hevc) but not
	// 10-bit of it. Only the 10-bit-capable plans copy a 10-bit source; others transcode it down.
	if copyVideo && f.TenBit() && plan != PlanHEVC10 && plan != PlanFull {
		copyVideo = false
	}
	return CopyPlan{
		CopyVideo: copyVideo,
		CopyAudio: a == "" || planCopyAudio[plan][a],
	}
}

// BroadcastVideoCodec resolves the one video codec a live session emits. EncodePlan describes a
// consumer bucket, not permission to switch codecs inside one stream: the HEVC plans are HEVC end
// to end, baseline is H.264 end to end, and the broad tuner plan follows the Channel's persisted
// broadcast codec.
func BroadcastVideoCodec(plan EncodePlan, channelCodec string) string {
	if plan.WantsHEVCOutput() || (plan == PlanFull && IsHEVCCodec(channelCodec)) {
		return "hevc"
	}
	return "h264"
}

// ConformCopyPlan narrows a codec-capability decision to the stable format of one live session.
// A source can be decodable by the audience and still be unsafe to splice into the current decoder
// timeline. Unknown properties deliberately fail toward transcode: extra work is recoverable; an
// in-stream format change is not.
func ConformCopyPlan(f MediaFormat, allowed CopyPlan, profile Profile, videoCodec string) CopyPlan {
	copyVideo := allowed.CopyVideo && sameBroadcastVideo(f, profile, videoCodec)
	copyAudio := allowed.CopyAudio && strings.EqualFold(strings.TrimSpace(f.AudioCodec), "aac") &&
		f.AudioChannels == 2
	return CopyPlan{CopyVideo: copyVideo, CopyAudio: copyAudio}
}

func sameBroadcastVideo(f MediaFormat, profile Profile, videoCodec string) bool {
	sourceCodec := strings.ToLower(strings.TrimSpace(f.VideoCodec))
	targetCodec := strings.ToLower(strings.TrimSpace(videoCodec))
	if IsHEVCCodec(sourceCodec) {
		sourceCodec = "hevc"
	}
	if IsHEVCCodec(targetCodec) {
		targetCodec = "hevc"
	}
	if sourceCodec == "" || sourceCodec != targetCodec || f.Width <= 0 || f.Height <= 0 ||
		f.Width != profile.Width || f.Height != profile.Height || f.FrameRate <= 0 ||
		math.Abs(f.FrameRate-float64(profile.Framerate)) > 0.01 {
		return false
	}
	// Every live encoder currently emits 8-bit SDR yuv420p. Copying a deeper/HDR source would
	// silently change decoder and colour state even when its codec and geometry happened to match.
	return strings.EqualFold(strings.TrimSpace(f.PixelFormat), "yuv420p") && !f.HDR()
}
