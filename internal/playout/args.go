package playout

import (
	"fmt"
	"strconv"
)

// ffmpeg argument construction for internal playout (§9.1).
//
// Args are built as a []string, never a shelled-out command line — a filter graph
// contains commas, colons and quotes, and handing that to a shell is how you get an
// injection or an unexplainable parse error.
//
// Shape borrowed from Tunarr/ErsatzTV (see docs/engineering/playout-prior-art.md):
// options are appended in POSITIONAL buckets, because ffmpeg is order-sensitive in ways
// that are easy to get wrong — an input option after `-i` applies to the *next* input,
// and a filter before its input is a parse error.
//
//	global → per-input (opts then -i) → filter → per-output → output target

// Encoder is a video encoder ffmpeg can use. Not an exhaustive list of what ffmpeg
// supports — only what playout offers, which is H.264 in software or via one of the
// three common hardware paths (§15 `playout.encoder`).
type Encoder string

// The families ErsatzTV maintains pipelines for, which is the breadth real deployments
// need: NVIDIA, Intel, AMD on both OSes, Apple Silicon, and ARM SBCs. Availability is never
// assumed from this list — `Detect` asks the local ffmpeg build and then tries each one.
const (
	EncoderSoftware     Encoder = "libx264"           // always available
	EncoderNVENC        Encoder = "h264_nvenc"        // NVIDIA
	EncoderQSV          Encoder = "h264_qsv"          // Intel Quick Sync
	EncoderVAAPI        Encoder = "h264_vaapi"        // Intel AND AMD on Linux
	EncoderAMF          Encoder = "h264_amf"          // AMD on Windows
	EncoderVideoToolbox Encoder = "h264_videotoolbox" // Apple Silicon / Intel Macs
	EncoderRKMPP        Encoder = "h264_rkmpp"        // Rockchip SBCs
	EncoderV4L2M2M      Encoder = "h264_v4l2m2m"      // Raspberry Pi, V4L2 stateful encoders
	EncoderVulkan       Encoder = "h264_vulkan"       // cross-vendor, newer
)

// Profile is the normalized output every program is encoded to.
//
// Normalization is not cosmetic — it is what makes the concat mechanism legal. The
// parent process concatenates per-program streams with `-c copy`, which is only valid
// if every child produced identical resolution, framerate, codec and pixel format.
// A child that quietly differs produces a stream players reject mid-program, and the
// symptom (a channel that dies a few minutes in) points nowhere near the cause.
type Profile struct {
	Width     int
	Height    int
	Framerate int
	// VideoBitrate in kbit/s. 0 = let the encoder choose (CRF-ish for software).
	VideoBitrate int
	Encoder      Encoder
	// AudioBitrate in kbit/s; audio is always AAC stereo 48kHz. Fixed deliberately:
	// a varying audio layout across programs breaks `-c copy` exactly like video does,
	// and ErsatzTV's comment about ac3 downmix reinit failures is the warning
	// (prior-art §5).
	AudioBitrate int
}

// DefaultProfile is a conservative 720p/25 H.264 target. Chosen for compatibility over
// quality: this is what a media server will remux to whatever the client wants, so the
// job here is to be universally decodable, not to look best.
func DefaultProfile() Profile {
	return Profile{
		Width: 1280, Height: 720, Framerate: 25,
		VideoBitrate: 4000, Encoder: EncoderSoftware, AudioBitrate: 128,
	}
}

// videoEncodeArgs returns the codec + rate-control args for the profile's encoder.
//
// One switch, dispatching per encoder — the shape both reference implementations use.
// Hardware paths differ enough in rate control that a shared "just set -b:v" would be
// wrong (nvenc wants a preset name libx264 doesn't have, and vaapi's quality scale is
// its own thing), but they share the bitrate/GOP helpers below.
func (p Profile) videoEncodeArgs() []string {
	args := []string{"-c:v", string(p.Encoder)}
	// Each family has its OWN preset vocabulary, and an unknown preset name fails at init
	// rather than being ignored. That is why this is a switch over families and not a
	// shared "-preset" line: libx264's "veryfast" is meaningless to nvenc, and nvenc's
	// "p4" is meaningless to everything else.
	switch p.Encoder {
	case EncoderSoftware:
		// veryfast because playout is realtime and a dropped frame is worse than a
		// slightly larger one. `-tune zerolatency` stops the encoder buffering frames it
		// would rather reorder — for live, latency beats compression.
		args = append(args, "-preset", "veryfast", "-tune", "zerolatency")
	case EncoderNVENC:
		// p4/hq rather than p1/ll: viewra found p1+ll produced visible grain artifacts
		// in tone-mapped content (prior-art, viewra §6).
		args = append(args, "-preset", "p4", "-tune", "hq")
	case EncoderAMF:
		// AMF speaks quality presets, not numbered ones.
		args = append(args, "-quality", "balanced")
	case EncoderQSV:
		// QSV's preset names overlap libx264's spelling but are its own enum.
		args = append(args, "-preset", "veryfast")
	case EncoderVAAPI, EncoderVideoToolbox, EncoderRKMPP, EncoderV4L2M2M, EncoderVulkan:
		// These take their quality from the bitrate args alone. Adding a preset from
		// another family's vocabulary is an init failure, and several of them (notably
		// v4l2m2m on a Pi) are strict about unknown options.
	}
	if p.VideoBitrate > 0 {
		kbps := strconv.Itoa(p.VideoBitrate) + "k"
		// maxrate+bufsize as well as -b:v: without a cap a live encoder can spike far
		// above target on a hard scene and blow a client's buffer.
		args = append(args, "-b:v", kbps, "-maxrate", kbps, "-bufsize",
			strconv.Itoa(p.VideoBitrate*2)+"k")
	}
	return append(args, p.gopArgs()...)
}

// gopArgs pins the keyframe interval to one per second.
//
// Every segment boundary must be a keyframe or a client joining mid-stream sees nothing
// until the next one. `-sc_threshold 0` disables scene-change detection, which would
// otherwise insert keyframes at unpredictable places and make segment durations vary —
// and a TARGETDURATION that lies is a player error, not a warning.
func (p Profile) gopArgs() []string {
	gop := strconv.Itoa(p.Framerate)
	return []string{"-g", gop, "-keyint_min", gop, "-sc_threshold", "0"}
}

// audioEncodeArgs is fixed AAC stereo 48kHz — see Profile.AudioBitrate.
func (p Profile) audioEncodeArgs() []string {
	return []string{
		"-c:a", "aac",
		"-b:a", strconv.Itoa(p.AudioBitrate) + "k",
		"-ac", "2",
		"-ar", "48000",
	}
}

// TestCardArgs builds the args for a synthetic test card: a captioned colour field with
// silent audio, muxed to MPEG-TS on stdout, forever.
//
// This is deliberately the first thing playout can do, because it proves the whole
// pipeline — encode, mux, pipe, serve — with NO library content, no scheduler and no
// media server. If the card plays, everything downstream of the encoder works.
//
// Three details are load-bearing, each learned from Tunarr's `ffmpegText.ts` rather than
// guessed (prior-art §5a):
//
//   - `anullsrc` gives a SILENT AUDIO TRACK. A video-only MPEG-TS is a classic cause of
//     a player refusing to play or showing no timeline. It is not optional.
//   - `-re` reads the synthetic source at realtime. Without it lavfi generates as fast
//     as the CPU allows and floods the pipe, racing ahead of wall-clock.
//   - `-stream_loop -1` so a generated source never EOFs and ends the channel.
func TestCardArgs(p Profile, fontFile, title, subtitle string) []string {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		// Progress on stdout as machine-readable key=value. NOT stderr scraping:
		// viewra parsed 4096-byte reads for "frame=" and a chunked read can split a
		// token across the boundary (prior-art, viewra §4).
		"-progress", "pipe:3", "-nostats",
	}
	// Video: a plain colour field, paced to realtime, looping forever.
	args = append(args,
		"-f", "lavfi", "-re", "-stream_loop", "-1",
		"-i", fmt.Sprintf("color=c=black:s=%dx%d:r=%d", p.Width, p.Height, p.Framerate),
	)
	// Audio: silence. Explicit layout + rate so it matches the encode profile exactly;
	// letting anullsrc default and then resampling is a needless filter.
	args = append(args,
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
	)
	if vf := drawTextFilter(fontFile, title, subtitle, p.Height); vf != "" {
		args = append(args, "-vf", vf)
	}
	args = append(args, p.videoEncodeArgs()...)
	args = append(args, p.audioEncodeArgs()...)
	// mpegts to stdout. `+initial_discontinuity` tells a downstream demuxer that the
	// first timestamps are not necessarily zero, which is true for anything joining a
	// live stream mid-flight (ErsatzTV sets the same flag).
	return append(args,
		"-f", "mpegts", "-mpegts_flags", "+initial_discontinuity", "pipe:1",
	)
}

// drawTextFilter centres a title, with an optional subtitle beneath it. Returns "" when
// there is no font — drawtext without a fontfile fails at init on a minimal image, so a
// missing font degrades to a plain colour field rather than killing the channel.
func drawTextFilter(fontFile, title, subtitle string, height int) string {
	if fontFile == "" || title == "" {
		return ""
	}
	size := height / 12 // scales with the profile instead of a fixed 30px
	f := fmt.Sprintf(
		"drawtext=fontfile=%s:fontsize=%d:fontcolor=white:x=(w-text_w)/2:y=(h-text_h)/2:text='%s'",
		fontFile, size, escapeDrawText(title))
	if subtitle != "" {
		f += fmt.Sprintf(
			",drawtext=fontfile=%s:fontsize=%d:fontcolor=white:x=(w-text_w)/2:y=(h+text_h+%d)/2:text='%s'",
			fontFile, size*2/3, size, escapeDrawText(subtitle))
	}
	return f
}

// escapeDrawText neutralises the characters that terminate or redirect a drawtext
// expression. A channel name is operator-supplied text arriving here from the database,
// so an unescaped apostrophe is a broken filter graph at best — and `:` introduces
// another filter option, which is worse than broken.
func escapeDrawText(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '\'', ':', '\\', '%':
			out = append(out, '\\', r)
		case '\n', '\r':
			out = append(out, ' ')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
