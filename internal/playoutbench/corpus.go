package playoutbench

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// CorpusName versions the generated corpus. Changing any clip below changes it, which invalidates
// baselines the same way a schema change does.
const CorpusName = "generated-v1"

// Every clip is synthetic (ffmpeg lavfi): redistributable, deterministic, and free of household
// media. Matroska is deliberate: the builder only uses minimal probing for a header-declared
// container, which is what the production inventory records.

// Clip is one generated corpus entry.
type Clip struct {
	Name string
	// Class groups clips whose numbers are reported together (`<metric>/<class>`); "" is a clip that
	// only feeds the audio or break checks.
	Class   string
	Seconds int
	// VideoSrc and AudioSrc are lavfi inputs; VideoEnc and AudioEnc are the output encode options.
	// They are kept apart because ffmpeg binds options to the next -i or output they precede.
	VideoSrc, VideoEnc, AudioSrc, AudioEnc []string
	// Break marks a clip that airs in the commercial-break sequence.
	Break bool
	// HDR clips may be refused by a host that cannot keep up; that is an outcome, not a failure.
	HDR bool
	// File overrides Path for a clip that was not generated; Window limits every encode to Seconds.
	File   string
	Window bool
	// VMAF asks for a quality score against the source (SDR only: a tone-mapped picture is not
	// comparable frame for frame).
	VMAF bool
}

// TargetLUFS is the level every generated clip is built at, the household target.
const TargetLUFS = -23.0

// sineGain puts a stereo 440 Hz sine at TargetLUFS: two channels of a sine of amplitude a measure
// 20*log10(a) - 0.7 LUFS after K-weighting.
const sineGain = 0.0766

var (
	sdr709 = []string{"-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709"}
	hdr10  = []string{"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc", "-pix_fmt", "yuv420p10le"}
)

// video is a testsrc2 input with temporal grain: a clean test card encodes at a fraction of real
// film's bitrate and would flatter every encoder.
func video(w, h int, rate, vf string) []string {
	chain := "noise=alls=4:allf=t"
	if vf != "" {
		chain += "," + vf
	}
	return []string{"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=%s,%s", w, h, rate, chain)}
}

// tone is a 48 kHz sine at the target loudness in the given layout; aformat pins the layout so an
// AC-3 clip really is 5.1 whatever the generator emits.
func tone(layout, af string) []string {
	src := fmt.Sprintf("sine=frequency=440:sample_rate=48000,volume=%g,aformat=channel_layouts=%s", sineGain, layout)
	if af != "" {
		src += "," + af
	}
	return []string{"-f", "lavfi", "-i", src}
}

func x264(extra ...string) []string {
	return append([]string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "20"}, extra...)
}

func x265(extra ...string) []string {
	return append([]string{"-c:v", "libx265", "-preset", "ultrafast", "-crf", "24", "-x265-params", "log-level=error"}, extra...)
}

func acodec(codec string, extra ...string) []string {
	return append([]string{"-c:a", codec}, extra...)
}

func fades(seconds int) (vf, af string) {
	return fmt.Sprintf("fade=t=in:st=0:d=0.5,fade=t=out:st=%d:d=0.5", seconds-1),
		fmt.Sprintf("afade=t=in:st=0:d=0.5,afade=t=out:st=%d:d=0.5", seconds-1)
}

// Corpus is the fixed clip list. Durations are long enough for a speed measurement and short enough
// for a CI runner; the start-latency runs seek into a clip and need only a second of it.
func Corpus() []Clip {
	clips := []Clip{
		{Name: "h264-1080p-25", Class: "h264-1080p", Seconds: 10, VMAF: true,
			VideoSrc: video(1920, 1080, "25", ""), VideoEnc: x264(sdr709...), AudioSrc: tone("5.1", ""), AudioEnc: acodec("eac3", "-b:a", "384k")},
		{Name: "h264-1080p-23.976", Seconds: 6,
			VideoSrc: video(1920, 1080, "24000/1001", ""), VideoEnc: x264(sdr709...), AudioSrc: tone("5.1", ""), AudioEnc: acodec("ac3", "-b:a", "384k")},
		{Name: "h264-1080p-29.97", Seconds: 6,
			VideoSrc: video(1920, 1080, "30000/1001", ""), VideoEnc: x264(sdr709...), AudioSrc: tone("stereo", ""), AudioEnc: acodec("aac", "-b:a", "160k")},
		{Name: "h264-1080i-29.97", Seconds: 6,
			VideoSrc: video(1920, 1080, "30000/1001", "interlace=scan=tff"),
			VideoEnc: x264(append(sdr709, "-flags", "+ilme+ildct", "-x264-params", "tff=1")...), AudioSrc: tone("stereo", ""), AudioEnc: acodec("aac")},
		{Name: "h264-1080p-10bit", Seconds: 6,
			VideoSrc: video(1920, 1080, "25", "format=yuv420p10le"), VideoEnc: x264(append(sdr709, "-pix_fmt", "yuv420p10le")...),
			AudioSrc: tone("stereo", ""), AudioEnc: acodec("aac")},
		{Name: "hevc-1080p-10bit", Class: "hevc-1080p", Seconds: 10,
			VideoSrc: video(1920, 1080, "25", "format=yuv420p10le"), VideoEnc: x265(append(sdr709, "-pix_fmt", "yuv420p10le")...),
			AudioSrc: tone("5.1", ""), AudioEnc: acodec("eac3")},
		{Name: "hevc-4k-hdr10", Class: "hdr-4k", Seconds: 10, HDR: true,
			VideoSrc: video(3840, 2160, "24000/1001", "format=yuv420p10le"), VideoEnc: x265(hdr10...),
			AudioSrc: tone("5.1", ""), AudioEnc: acodec("eac3")},
		// TrueHD-like: 7.1 through the experimental TrueHD encoder. The builder must map a lossless
		// multichannel track to the stereo AAC output like any other layout.
		{Name: "h264-1080p-truehd", Seconds: 6,
			VideoSrc: video(1920, 1080, "25", ""), VideoEnc: x264(sdr709...), AudioSrc: tone("7.1", ""), AudioEnc: acodec("truehd", "-strict", "-2")},
	}
	// The commercial-break sequence: four spots of different geometry, cadence, codec and audio
	// layout, each faded in and out, all at the target loudness.
	spots := []struct {
		name         string
		w, h         int
		rate, layout string
		hevc         bool
		audio        []string
	}{
		{"spot-720p-25", 1280, 720, "25", "stereo", false, acodec("aac")},
		{"spot-1080p-29.97", 1920, 1080, "30000/1001", "5.1", false, acodec("ac3", "-b:a", "384k")},
		{"spot-480p-25", 854, 480, "25", "stereo", false, acodec("aac")},
		{"spot-1080p-hevc", 1920, 1080, "25", "stereo", true, acodec("aac")},
	}
	const spotSeconds = 6
	vf, af := fades(spotSeconds)
	for _, s := range spots {
		enc := x264(sdr709...)
		if s.hevc {
			enc = x265(sdr709...)
		}
		clips = append(clips, Clip{Name: s.name, Seconds: spotSeconds, Break: true,
			VideoSrc: video(s.w, s.h, s.rate, vf), VideoEnc: enc, AudioSrc: tone(s.layout, af), AudioEnc: s.audio})
	}
	return clips
}

// Args is the ffmpeg command that writes the clip to path.
func (c Clip) Args(path string) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-t", strconv.Itoa(c.Seconds)}
	args = append(args, c.VideoSrc...)
	args = append(args, c.AudioSrc...)
	args = append(args, "-map", "0:v:0", "-map", "1:a:0")
	args = append(args, c.VideoEnc...)
	args = append(args, c.AudioEnc...)
	return append(args, "-f", "matroska", path)
}

// Path is where the clip lives under dir.
func (c Clip) Path(dir string) string {
	if c.File != "" {
		return c.File
	}
	return filepath.Join(dir, c.Name+".mkv")
}

// Generate writes every clip under dir, skipping those already present. A clip whose encoder this
// ffmpeg build lacks is returned in skipped with the reason rather than failing the bench.
func Generate(ctx context.Context, ffmpeg, dir string) (clips []Clip, skipped map[string]string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	skipped = map[string]string{}
	for _, c := range Corpus() {
		path := c.Path(dir)
		if _, statErr := os.Stat(path); statErr == nil {
			clips = append(clips, c)
			continue
		}
		part := path + ".part"
		out, runErr := exec.CommandContext(ctx, ffmpeg, c.Args(part)...).CombinedOutput()
		if runErr != nil {
			_ = os.Remove(part)
			skipped[c.Name] = firstLine(string(out), runErr)
			continue
		}
		if err := os.Rename(part, path); err != nil {
			return nil, nil, err
		}
		clips = append(clips, c)
	}
	return clips, skipped, nil
}

func firstLine(out string, err error) string {
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return err.Error()
}

// filmWindow is how much of an open film the bench encodes: the film is a realistic-content check
// (real grain, real motion), not a full-length transcode.
const filmWindow = 30

// Films returns a clip for every video file in dir (the open films fetched by
// scripts/playout-bench-open-films.sh). Each is its own class, `film-<stem>`, so its numbers sit
// beside the generated classes and are diffed against the baseline like them.
func Films(dir string) ([]Clip, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var films []Clip
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || (ext != ".mov" && ext != ".mkv" && ext != ".mp4" && ext != ".webm") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		films = append(films, Clip{Name: stem, Class: "film-" + stem, Seconds: filmWindow, File: filepath.Join(dir, e.Name()), Window: true})
	}
	return films, nil
}

// frames is the -frames limit for a windowed clip, 0 (the whole clip) otherwise.
func (c Clip) frames(fps int) int {
	if c.Window {
		return c.Seconds * fps
	}
	return 0
}
