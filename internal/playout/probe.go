package playout

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// All ffprobe use lives here (§9.1). One invocation, one stream model, three consumers.
//
// Playout asks three questions of a source, and they used to be three separate ffprobe calls with
// three JSON shapes and three copies of the shell-out: which audio track to map (audio.go), the
// tracks the Watch pickers list (tracks.go), and the codec the copy decision needs (format,
// directplay/copyplan). They all read the SAME `ffprobe -show_streams` output — so this file runs
// it once into one `probedStream` slice, and each consumer is a pure function over that. Fewer
// execs, one JSON shape, no drift between three parsers of the same data.

// probedStream is one stream from `ffprobe -show_streams`, carrying every field any consumer needs
// (the superset). Extend this — not a parallel struct — when a new consumer needs another field.
type probedStream struct {
	Index         int    `json:"index"`
	CodecType     string `json:"codec_type"`
	CodecName     string `json:"codec_name"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	AvgFrameRate  string `json:"avg_frame_rate"` // "24000/1001" — a rational, parsed to fps
	PixFmt        string `json:"pix_fmt"`        // "yuv420p10le" → 10-bit signal
	ColorTransfer string `json:"color_transfer"` // "smpte2084"/"arib-std-b67" → HDR
	Channels      int    `json:"channels"`       // audio channel count (2, 6, …)
	SampleRate    string `json:"sample_rate"`    // audio Hz, emitted as a JSON string
	Tags          struct {
		Language string `json:"language"`
		Title    string `json:"title"`
	} `json:"tags"`
}

// probedPacket carries only what decides whether a container is hiding pre-cut video. ffprobe's
// `D` flag means the packet is marked discard (for example by an MP4 edit list); when that packet
// also precedes timestamp zero, copying it into MPEG-TS can make the hidden GOP visible.
type probedPacket struct {
	StreamIndex int    `json:"stream_index"`
	PTS         string `json:"pts_time"`
	Flags       string `json:"flags"`
}

// probedFormat is the container-level `-show_format` slice — one per file, not per stream.
type probedFormat struct {
	FormatName string `json:"format_name"` // "matroska,webm", "mov,mp4,…"
	Duration   string `json:"duration"`    // seconds, as a string
	BitRate    string `json:"bit_rate"`    // overall bit/s, as a string
}

// probed is the full result of one ffprobe call: every stream plus the container format.
type probed struct {
	Streams []probedStream `json:"streams"`
	Packets []probedPacket `json:"packets"`
	Format  probedFormat   `json:"format"`
}

// runFFprobe shells out once and returns the source's streams + container format. The field
// selection is the SUPERSET every consumer needs, so a single call feeds all of them — and captures
// everything a probe can cheaply tell us (codecs, dimensions, framerate, HDR/bit-depth, channels,
// container, duration, bitrate) so later features need no second probe. `-v error` keeps ffprobe's
// chatter off our stderr; a probe failure is returned, never fatal — callers degrade (audio →
// track 0, format → transcode-required, tracks → empty list).
func runFFprobe(ctx context.Context, bin, input string, inspectPackets bool) (probed, error) {
	entries := "stream=index,codec_type,codec_name,width,height,avg_frame_rate,pix_fmt,color_transfer,channels,sample_rate" +
		":stream_tags=language,title" +
		":format=format_name,duration,bit_rate"
	args := []string{
		"-v", "error",
	}
	if inspectPackets {
		// Packet inspection is bounded: enough to reach the first video packet even when audio is
		// interleaved first, without turning a format probe into a scan of the whole asset. Audio and
		// track-list probes do not pay this boundary-safety cost.
		args = append(args, "-read_intervals", "%+#128")
		entries += ":packet=stream_index,pts_time,flags"
	}
	args = append(args, "-show_entries", entries, "-of", "json", input)
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.Output()
	if err != nil {
		return probed{}, err
	}
	return parseProbeJSON(out)
}

// parseProbeJSON decodes ffprobe's JSON — split from the exec so tests exercise the real struct
// decode (the tag mapping) over fixture bytes without shelling out.
func parseProbeJSON(raw []byte) (probed, error) {
	var parsed probed
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return probed{}, err
	}
	return parsed, nil
}

// ffprobeBesideFFmpeg maps an ffmpeg path to the ffprobe next to it.
//
// `playout.ffmpeg_path` is the only path setting operators have, and the two binaries ship
// together — a second setting would be a knob whose only correct value is derivable from the first.
func ffprobeBesideFFmpeg(ffmpegPath string) string {
	if ffmpegPath == "" || ffmpegPath == "ffmpeg" {
		return "ffprobe"
	}
	dir, base := filepath.Split(ffmpegPath)
	return filepath.Join(dir, strings.Replace(base, "ffmpeg", "ffprobe", 1))
}

// --- Consumer 1: audio-track selection (audio.go's PickAudioTrack input) ------------------------

// FFprobeAudioNextTo returns an AudioProber built on the shared probe. The audio streams, in the
// order ffmpeg numbers them — which is what PickAudioTrack's audio-relative index needs.
func FFprobeAudioNextTo(ffmpegPath string) AudioProber {
	bin := ffprobeBesideFFmpeg(ffmpegPath)
	return func(ctx context.Context, input string) ([]AudioTrack, error) {
		p, err := runFFprobe(ctx, bin, input, false)
		if err != nil {
			return nil, err
		}
		return audioTracksOf(p.Streams), nil
	}
}

// audioTracksOf extracts the audio streams in order — pure, testable over a stream slice.
func audioTracksOf(streams []probedStream) []AudioTrack {
	var out []AudioTrack
	for _, s := range streams {
		if s.CodecType == "audio" {
			out = append(out, AudioTrack{Language: s.Tags.Language})
		}
	}
	return out
}

// --- Consumer 2: the Watch pickers' tracks (tracks.go's MediaTracks) ----------------------------

// FFprobeTracksNextTo returns a TrackProber built on the shared probe — the audio + subtitle tracks
// the Watch surface offers.
func FFprobeTracksNextTo(ffmpegPath string) TrackProber {
	bin := ffprobeBesideFFmpeg(ffmpegPath)
	return func(ctx context.Context, input string) (MediaTracks, error) {
		p, err := runFFprobe(ctx, bin, input, false)
		if err != nil {
			return MediaTracks{}, err
		}
		return tracksOf(p.Streams), nil
	}
}

// tracksOf buckets streams into audio + subtitle tracks, numbering each type from zero (the
// per-type index a later `-map 0:a:N` / `0:s:N` uses). Pure, testable over a stream slice.
func tracksOf(streams []probedStream) MediaTracks {
	var tracks MediaTracks
	audioN, subN := 0, 0
	for _, s := range streams {
		t := Track{
			Language: strings.ToLower(strings.TrimSpace(s.Tags.Language)),
			Title:    strings.TrimSpace(s.Tags.Title),
		}
		switch s.CodecType {
		case "audio":
			t.Index = audioN
			audioN++
			tracks.Audio = append(tracks.Audio, t)
		case "subtitle":
			t.Index = subN
			subN++
			tracks.Subtitles = append(tracks.Subtitles, t)
		}
	}
	return tracks
}

// --- Consumer 3: the copy/transcode decision (copyplan.go's MediaFormat) ------------------------

// FFprobeFormatNextTo returns a FormatProber built on the shared probe — the full MediaFormat for
// PlanCopy and any later feature that needs it.
func FFprobeFormatNextTo(ffmpegPath string) FormatProber {
	bin := ffprobeBesideFFmpeg(ffmpegPath)
	return func(ctx context.Context, input string) (MediaFormat, error) {
		p, err := runFFprobe(ctx, bin, input, true)
		if err != nil {
			return MediaFormat{}, err
		}
		return formatOf(p), nil
	}
}

// formatOf builds a MediaFormat from the probe: the first video stream (codec + geometry + framerate
// + pixel format + HDR transfer), the first audio stream (codec + channels), and the container-level
// format/duration/bitrate. "First stream" because playout maps one video + one primary audio; track
// SELECTION among audio streams is a separate concern (audio.go). Pure, testable over probe output.
func formatOf(p probed) MediaFormat {
	var f MediaFormat
	videoIndex := -1
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			if f.VideoCodec == "" {
				videoIndex = s.Index
				f.VideoCodec = strings.ToLower(s.CodecName)
				f.Width, f.Height = s.Width, s.Height
				f.FrameRate = parseRational(s.AvgFrameRate)
				f.PixelFormat = strings.ToLower(s.PixFmt)
				f.ColorTransfer = s.ColorTransfer
			}
		case "audio":
			if f.AudioCodec == "" {
				f.AudioCodec = strings.ToLower(s.CodecName)
				f.AudioChannels = s.Channels
				f.AudioSampleRate = int(parseInt(s.SampleRate))
			}
		}
	}
	f.Container = p.Format.FormatName
	f.Duration = parseFloat(p.Format.Duration)
	f.Bitrate = parseInt(p.Format.BitRate)
	for _, packet := range p.Packets {
		if packet.StreamIndex != videoIndex {
			continue
		}
		f.VideoPreroll = parseFloat(packet.PTS) < 0 && strings.Contains(packet.Flags, "D")
		break
	}
	return f
}

// parseRational turns ffprobe's "24000/1001" framerate into fps. Returns 0 on anything unparseable
// or a zero denominator (ffprobe emits "0/0" for streams with no framerate).
func parseRational(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		return parseFloat(s)
	}
	n, d := parseFloat(num), parseFloat(den)
	if d == 0 {
		return 0
	}
	return n / d
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

func parseInt(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
