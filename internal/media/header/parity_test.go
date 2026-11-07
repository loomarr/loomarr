package header_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/playout"
)

// A parity case is an ffmpeg command that writes one file. mustRead says the native reader has to
// describe it (the common shapes of a household library); otherwise it may decline, and when it
// does answer it must still agree with ffprobe.
type parityCase struct {
	name     string
	file     string
	args     []string
	mustRead bool
}

func parityCases(dir string) []parityCase {
	meta := filepath.Join(dir, "chapters.txt")
	subs := filepath.Join(dir, "subs.srt")
	src := func(seconds string) []string {
		return []string{"-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=" + seconds,
			"-f", "lavfi", "-i", "sine=f=440:r=48000:d=" + seconds}
	}
	return []parityCase{
		{
			name: "matroska h264, two audio tracks, a forced subtitle, chapters", file: "h264.mkv", mustRead: true,
			args: append(append(src("4"), "-i", subs, "-i", meta), "-map", "0:v", "-map", "1:a", "-map", "1:a", "-map", "2:s",
				"-map_metadata", "3", "-map_chapters", "3",
				"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
				"-c:a:0", "aac", "-c:a:1", "ac3", "-ac:a:1", "6", "-c:s", "srt",
				"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=fre", "-metadata:s:a:1", "title=Commentary",
				"-metadata:s:s:0", "language=spa", "-disposition:a:0", "default", "-disposition:a:1", "0",
				"-disposition:s:0", "forced"),
		},
		{
			name: "matroska h264 at 23.976", file: "ntsc.mkv", mustRead: true,
			args: []string{"-f", "lavfi", "-i", "testsrc2=s=320x240:r=24000/1001:d=3", "-c:v", "libx264",
				"-preset", "ultrafast", "-pix_fmt", "yuv420p"},
		},
		{
			name: "matroska hevc 10-bit hdr10", file: "hdr.mkv", mustRead: true,
			args: []string{"-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=2", "-c:v", "libx265", "-preset", "ultrafast",
				"-pix_fmt", "yuv420p10le", "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc",
				"-x265-params", "log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc"},
		},
		{
			name: "mp4 h264 with the moov at the end", file: "late.mp4", mustRead: true,
			args: append(src("3"), "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac"),
		},
		{
			name: "mp4 hevc faststart", file: "fast.mp4", mustRead: true,
			args: append(src("2"), "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error",
				"-tag:v", "hvc1", "-c:a", "aac", "-movflags", "+faststart"),
		},
		{
			name: "mp4 e-ac-3", file: "eac3.mp4",
			args: append(src("2"), "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "eac3", "-ac", "6"),
		},
		{
			name: "webm vp9 and opus", file: "vp9.webm",
			args: append(src("2"), "-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-c:a", "libopus"),
		},
		{
			name: "matroska av1", file: "av1.mkv",
			args: []string{"-f", "lavfi", "-i", "testsrc2=s=320x240:r=25:d=1", "-c:v", "libsvtav1", "-preset", "12"},
		},
		{
			name: "matroska mpeg-2", file: "mpeg2.mkv",
			args: append(src("2"), "-c:v", "mpeg2video", "-c:a", "mp2"),
		},
		{
			name: "matroska interlaced h264", file: "interlaced.mkv",
			args: []string{"-f", "lavfi", "-i", "testsrc2=s=720x480:r=30000/1001:d=2", "-c:v", "libx264", "-preset", "ultrafast",
				"-pix_fmt", "yuv420p", "-flags", "+ildct+ilme", "-x264-params", "interlaced=1:tff=1"},
		},
		{
			name: "avi falls back", file: "old.avi",
			args: append(src("2"), "-c:v", "mpeg4", "-c:a", "mp3"),
		},
	}
}

func writeParityInputs(t *testing.T, dir string) {
	t.Helper()
	srt := "1\n00:00:00,500 --> 00:00:01,500\nHola\n\n"
	if err := os.WriteFile(filepath.Join(dir, "subs.srt"), []byte(srt), 0o600); err != nil {
		t.Fatal(err)
	}
	meta := ";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=2000\ntitle=One\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=2000\nEND=4000\ntitle=Two\n"
	if err := os.WriteFile(filepath.Join(dir, "chapters.txt"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The native reader answers what ffprobe answers, field by field, for every stream playout and
// the Library read — or declines and lets ffprobe answer.
func TestHeaderFactsMatchFFprobe(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	writeParityInputs(t, dir)
	probe := playout.FFprobeSourceNextTo(ffmpeg)
	for _, c := range parityCases(dir) {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, c.file)
			args := append([]string{"-nostdin", "-v", "error", "-y"}, c.args...)
			if out, err := exec.Command(ffmpeg, append(args, path)...).CombinedOutput(); err != nil {
				if strings.Contains(string(out), "Unknown encoder") || strings.Contains(string(out), "Encoder not found") {
					t.Skipf("encoder unavailable: %s", out)
				}
				t.Fatalf("fixture: %v: %s", err, out)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			info, _ := f.Stat()
			got, ok := header.Read(f, info.Size())
			if !ok {
				if c.mustRead {
					t.Fatalf("the native reader declined %s", c.file)
				}
				t.Logf("declined; ffprobe answers")
				return
			}
			want, err := probe(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			frameColour(t, path, &want)
			compareHeader(t, got, want)
		})
	}
}

// frameColour replaces ffprobe's stream-level colour with the first decoded frame's. The stream
// summary trusts a Matroska Colour element over the bitstream: a file whose Colour carries only
// the matrix reads as SDR there while every frame is PQ (#2151 step 3). The frame is the truth
// playout's picture has.
func frameColour(t *testing.T, path string, obs *playout.SourceObservation) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0", "-read_intervals", "%+#1",
		"-show_entries", "frame=color_transfer,color_primaries,color_space", "-of", "json", path).Output()
	if err != nil {
		t.Fatalf("frame colour: %v", err)
	}
	var parsed struct {
		Frames []struct {
			Transfer  string `json:"color_transfer"`
			Primaries string `json:"color_primaries"`
			Matrix    string `json:"color_space"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil || len(parsed.Frames) == 0 {
		return
	}
	known := func(s string) string {
		if s == "unknown" || s == "reserved" {
			return ""
		}
		return s
	}
	f := parsed.Frames[0]
	for i := range obs.Streams {
		if obs.Streams[i].Kind == "video" {
			s := &obs.Streams[i]
			s.ColorTransfer, s.ColorPrimaries, s.ColorSpace = known(f.Transfer), known(f.Primaries), known(f.Matrix)
			s.HDR = s.ColorTransfer == "smpte2084" || s.ColorTransfer == "arib-std-b67"
			return
		}
	}
}

func compareHeader(t *testing.T, got header.Header, want playout.SourceObservation) {
	t.Helper()
	if got.Container != want.Container {
		t.Errorf("container %q, ffprobe %q", got.Container, want.Container)
	}
	if d := got.DurationMillis - want.DurationMillis; d < -1 || d > 1 {
		t.Errorf("duration %d ms, ffprobe %d ms", got.DurationMillis, want.DurationMillis)
	}
	if want.Bitrate > 0 {
		if d := float64(got.Bitrate-want.Bitrate) / float64(want.Bitrate); d < -0.005 || d > 0.005 {
			t.Errorf("bitrate %d, ffprobe %d", got.Bitrate, want.Bitrate)
		}
	}
	if len(got.Streams) != len(want.Streams) {
		t.Fatalf("%d streams, ffprobe %d: %+v", len(got.Streams), len(want.Streams), got.Streams)
	}
	for i, g := range got.Streams {
		w := want.Streams[i]
		check := func(field string, gv, wv any) {
			if gv != wv {
				t.Errorf("stream %d %s: %v, ffprobe %v", i, field, gv, wv)
			}
		}
		check("index", g.Index, w.Index)
		check("kind", string(g.Kind), w.Kind)
		check("codec", g.Codec, w.Codec)
		check("language", g.Language, w.Language)
		check("title", g.Title, w.Title)
		check("default", g.Disposition.Default, w.Default)
		check("forced", g.Disposition.Forced, w.Forced)
		switch g.Kind {
		case inventory.StreamVideo:
			check("width", g.Width, w.Width)
			check("height", g.Height, w.Height)
			check("frame rate", g.FrameRate, w.FrameRate)
			check("pixel format", g.PixelFormat, w.PixelFormat)
			check("transfer", g.ColorTransfer, w.ColorTransfer)
			check("primaries", g.ColorPrimaries, w.ColorPrimaries)
			check("matrix", g.ColorSpace, w.ColorSpace)
			check("hdr", g.HDR, w.HDR)
			check("interlaced", g.Interlaced, w.Interlaced)
			if g.Profile != "" {
				check("profile", g.Profile, w.Profile)
			}
			if g.Level != "" {
				check("level", g.Level, w.Level)
			}
		case inventory.StreamAudio:
			check("channels", g.Channels, w.Channels)
			check("sample rate", g.SampleRate, w.SampleRate)
			if g.Profile != "" {
				check("profile", g.Profile, w.Profile)
			}
		}
	}
}

// Most Matroska files are described from ONE read of the first 64 KiB; a cold read on a NAS is
// one disk seek, so this is the number that sets how fast a first import is.
func TestMatroskaHeaderIsOneRead(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	path := filepath.Join(t.TempDir(), "one.mkv")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=20",
		"-f", "lavfi", "-i", "sine=f=440:r=48000:d=20", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-b:v", "2M", "-c:a", "aac", path).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, out)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	info, _ := f.Stat()
	counted := &countingReader{r: f}
	if _, ok := header.Read(counted, info.Size()); !ok {
		t.Fatal("declined a plain h264 matroska file")
	}
	if counted.reads != 1 || counted.bytes > 64<<10 {
		t.Errorf("%d reads, %d bytes; want one read of at most 64 KiB", counted.reads, counted.bytes)
	}
}

type countingReader struct {
	r            io.ReaderAt
	reads, bytes int
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	c.reads++
	n, err := c.r.ReadAt(p, off)
	c.bytes += n
	return n, err
}

// Probe falls back for a container the native readers decline, and says which reader answered.
func TestProbeFallsBackAndSaysWhichReaderAnswered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.ts")
	if err := os.WriteFile(path, make([]byte, 188*4), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	h, method, err := header.Probe(context.Background(), path, func(context.Context, string) (header.Header, error) {
		called = true
		return header.Header{Container: "mpegts", DurationMillis: 1000}, nil
	})
	if err != nil || !called || method != header.MethodFFprobe || h.Container != "mpegts" {
		t.Fatalf("Probe = %+v, %q, %v (fallback called %v)", h, method, err, called)
	}
	if _, _, err := header.Probe(context.Background(), path, nil); err == nil {
		t.Fatal("no fallback must be an error, not empty facts")
	}
}
