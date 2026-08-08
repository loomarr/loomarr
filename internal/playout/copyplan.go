package playout

import (
	"context"
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

// Target is a playback destination's codec tolerance — what it can play without transcoding.
type Target int

const (
	// TargetBrowser is the in-app HLS player (hls.js / native / a future mobile app). The safe
	// baseline MSE and Safari/iOS both play is H.264 video + AAC audio. HEVC plays only on some
	// Apple hardware, so it is NOT in the browser baseline; surround audio (EAC3/AC3) is not
	// broadly decodable in-browser either.
	TargetBrowser Target = iota
	// TargetMediaServer is a media server tuner (Emby/Jellyfin) or a general TV client, which
	// handles a broader set directly and re-transcodes anything it cannot itself play.
	TargetMediaServer
)

// String renders a target as the token used on the wire (the `?target=` query on the playlist and
// program URLs) and in a session key. Stable, lowercase, and the inverse of ParseTarget.
func (t Target) String() string {
	switch t {
	case TargetBrowser:
		return "browser"
	default:
		return "mediaserver"
	}
}

// ParseTarget maps the wire token back to a Target. Unknown or empty ⇒ TargetMediaServer — the
// safe default: it is the broader capability set (a media-server tuner ingests more than a
// browser), and it is what the tuner path has always produced, so an un-parameterised request
// (an old client, a hand-typed URL) behaves exactly as before. The browser narrows EXPLICITLY.
func ParseTarget(s string) Target {
	if s == "browser" {
		return TargetBrowser
	}
	return TargetMediaServer
}

// directPlayVideo / directPlayAudio are the codecs each target plays without a transcode.
var directPlayVideo = map[Target]map[string]bool{
	TargetBrowser:     {"h264": true},
	TargetMediaServer: {"h264": true, "hevc": true, "h265": true},
}

var directPlayAudio = map[Target]map[string]bool{
	TargetBrowser:     {"aac": true},
	TargetMediaServer: {"aac": true, "ac3": true, "eac3": true, "mp3": true},
}

// CopyPlan is the per-stream copy/transcode decision for one source against one target. It maps
// directly onto ffmpeg codec flags: CopyVideo → `-c:v copy` else a video encoder; CopyAudio →
// `-c:a copy` else `-c:a aac`.
type CopyPlan struct {
	CopyVideo bool
	CopyAudio bool
}

// DirectPlay reports whether NOTHING needs transcoding — both streams copy. The fastest, cheapest
// path (no encode at all).
func (p CopyPlan) DirectPlay() bool { return p.CopyVideo && p.CopyAudio }

// PlanCopy decides, per stream, what can be copied for `target` — the core decision, pure and
// testable without ffmpeg.
//
//   - Video copies when the target plays its codec (h264 for a browser). Otherwise it transcodes.
//   - Audio copies when the target plays its codec, OR when there is no audio. Otherwise the audio
//     alone transcodes (to AAC) — the video is untouched. This is what makes an h264 + EAC3 file
//     cost only a cheap audio encode for the browser, not a full video re-encode.
func PlanCopy(f MediaFormat, target Target) CopyPlan {
	v := strings.ToLower(strings.TrimSpace(f.VideoCodec))
	a := strings.ToLower(strings.TrimSpace(f.AudioCodec))
	return CopyPlan{
		CopyVideo: directPlayVideo[target][v],
		CopyAudio: a == "" || directPlayAudio[target][a],
	}
}
