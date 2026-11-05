package playout

import (
	"context"
	"strings"
)

// MediaFormat is everything a probe tells us about a source that playout might use — the class
// each item airs as (formats.go) needs the codecs; the rest (dimensions, framerate, HDR, bit depth,
// duration, bitrate, container) is captured in the SAME probe so later features (framerate-aware
// transcode, HDR tone-mapping, quality display, subtitle burn-in) need no second ffprobe. Probe
// once, keep it all. It is probed from the actual media (probe.go), never the media server's
// metadata.
type MediaFormat struct {
	// Video.
	VideoCodec    string // e.g. "h264", "hevc", "mpeg2video" — lowercased; empty when no video
	Width         int
	Height        int
	FrameRate     float64 // frames/sec, from avg_frame_rate; 0 when unknown
	PixelFormat   string  // e.g. "yuv420p", "yuv420p10le" — the 10le suffix signals 10-bit
	ColorTransfer string  // e.g. "smpte2084" (HDR10/PQ), "arib-std-b67" (HLG); "" for SDR
	// VideoPreroll means the container hides a negative-timestamp video packet behind a discard
	// edit. MPEG-TS cannot carry that edit safely, so live playout must decode from the true start
	// instead of copying the hidden preceding GOP into the channel.
	VideoPreroll bool
	// Interlaced is the inventory measurement's field-order fact; the transcode deinterlaces it.
	Interlaced bool
	// Active is the measured picture area inside the coded frame (letterbox and pillarbox bars
	// excluded), in source pixels; zero means the whole frame. The watermark anchors to it.
	Active Rect

	// Audio (the first/primary track — track SELECTION is a separate concern, see audio.go).
	AudioCodec      string // e.g. "aac", "eac3", "ac3" — lowercased; empty when no audio
	AudioChannels   int    // 2 = stereo, 6 = 5.1, …; 0 when unknown
	AudioSampleRate int    // Hz; the live copy format is fixed at 48000

	// Container + overall.
	Container string  // format_name, e.g. "matroska,webm", "mov,mp4,…"; "" when unknown
	Duration  float64 // seconds; 0 when unknown
	Bitrate   int64   // overall bit/s; 0 when unknown
}

// IsHEVCCodec reports whether a codec-name string names HEVC/H.265. ffprobe emits "hevc" but
// some sources/containers label the same codec "h265", so both count — the single place that
// equivalence lives (used by the channel-codec measurement).
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

// PQ reports whether the video is HDR10-style PQ (SMPTE ST 2084), including Dolby Vision's HDR10
// base layer. An HDR10 output passes it through; HLG is converted like SDR.
func (f MediaFormat) PQ() bool {
	return strings.ToLower(f.ColorTransfer) == "smpte2084"
}

// FormatProber reports a source's MediaFormat. An interface so callers are testable without exec,
// mirroring TrackProber. The concrete prober (FFprobeFormatNextTo) lives in probe.go.
type FormatProber func(ctx context.Context, input string) (MediaFormat, error)
