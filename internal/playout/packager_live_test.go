//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout/packager"
)

// TestPackagerStitchesMixedFormatItems runs the real chain the channel packager owns: the phase-1a
// builder's per-item fMP4 command, one encoder per slot, stitched onto one timeline. The sources
// differ in geometry, frame rate and sample rate, as a programme and its commercials do. The
// packaged output must have every video delta one output frame, every audio delta one AAC frame,
// no decode errors, and no decoder re-initialisation (one init segment for the whole channel).
func TestPackagerStitchesMixedFormatItems(t *testing.T) {
	dir := t.TempDir()
	out := OutputProfile{Width: 640, Height: 360, FPS: 30, Quality: 28, TargetKbps: 800, MaxKbps: 1200, GOPSeconds: 1, AudioKbps: 128}
	host := HostFor(EncoderSoftware, true, GPUFilters{})
	type src struct {
		name, lavfi string
		w, h, fps   int
		rate        int
	}
	sources := []src{
		{"prog", "testsrc2=s=1280x720:r=25", 1280, 720, 25, 44100},
		{"ad1", "smptebars=s=720x480:r=24000/1001", 720, 480, 24, 48000},
		{"ad2", "mandelbrot=s=854x480:r=50", 854, 480, 50, 32000},
		{"slate", "color=c=black:s=640x360:r=30", 640, 360, 30, 48000},
	}
	paths := map[string]string{}
	formats := map[string]MediaFormat{}
	for _, s := range sources {
		p := filepath.Join(dir, s.name+".mkv")
		ffmpegRun(t, "-f", "lavfi", "-i", s.lavfi, "-f", "lavfi", "-i", "sine=f=440:r="+strconv.Itoa(s.rate),
			"-t", "6", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", p)
		paths[s.name] = p
		formats[s.name] = MediaFormat{VideoCodec: "h264", Width: s.w, Height: s.h, FrameRate: float64(s.fps),
			PixelFormat: "yuv420p", AudioCodec: "aac", AudioChannels: 1, AudioSampleRate: s.rate, Container: "matroska,webm"}
	}
	args := func(name string, seek time.Duration, slot packager.Slot) []string {
		p, err := Build(host, formats[name], out)
		if err != nil {
			t.Fatal(err)
		}
		return p.FragmentArgs(paths[name], seek, slot.Offset, slot.Frames, slot.AudioFrames, out.FPS, 0)
	}
	slateOut, err := exec.Command("ffmpeg", args("slate", 0, packager.Slot{Frames: int64(out.FPS), AudioFrames: 47})...).Output()
	if err != nil {
		t.Fatalf("slate encode: %v", err)
	}
	slate, err := packager.NewSlate(slateOut)
	if err != nil {
		t.Fatal(err)
	}

	plan := []struct {
		name string
		seek time.Duration
		dur  time.Duration
	}{{"prog", 1500 * time.Millisecond, 2500 * time.Millisecond}, {"ad1", 0, 2 * time.Second}, {"ad2", time.Second, 3 * time.Second}}
	finished := make(chan struct{})
	n := 0
	sched := func(ctx context.Context, _ time.Time) (packager.Item, error) {
		if n == len(plan) {
			close(finished)
			<-ctx.Done()
			return packager.Item{}, ctx.Err()
		}
		it := plan[n]
		n++
		return packager.Item{Label: it.name, Duration: it.dur, Open: func(ctx context.Context, slot packager.Slot) (io.ReadCloser, error) {
			cmd := exec.CommandContext(ctx, "ffmpeg", args(it.name, it.seek, slot)...)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				return nil, err
			}
			return stdout, cmd.Start()
		}}, nil
	}
	segDir := filepath.Join(dir, "hls")
	if err := os.Mkdir(segDir, 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := packager.New(packager.Config{FPS: out.FPS, Dir: segDir, RunAhead: time.Hour, FirstItemWait: 10 * time.Second,
		SlateLead: time.Nanosecond}, sched, func(context.Context) (*packager.Slate, error) { return slate, nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = p.Run(ctx) }()
	select {
	case <-finished:
	case <-time.After(60 * time.Second):
		t.Fatal("items did not finish")
	}
	stop()
	<-done
	if s := p.Stats(); s.Items != 3 || s.Slates != 0 || s.DecoderMismatch != 0 {
		t.Fatalf("stats %+v: every item must join the channel's one init segment", s)
	}

	// Concatenate init + segments in order: what a player downloads.
	segs, _ := filepath.Glob(filepath.Join(segDir, "seg*.m4s"))
	var all bytes.Buffer
	all.Write(p.Init())
	for _, s := range segs {
		b, err := os.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	channel := filepath.Join(dir, "channel.mp4")
	if err := os.WriteFile(channel, all.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if n := len(segs); n != 8 { // 2.5 + 2 + 3 s: 3 + 2 + 3 segments (the first slot ends mid-GOP)
		t.Errorf("%d segments, want 8", n)
	}
	for stream, want := range map[string]int{"v:0": 3000, "a:0": 1024} {
		deltas := packetDeltas(t, channel, stream)
		for i, d := range deltas {
			if d != want {
				t.Fatalf("%s packet %d delta %d, want %d (a gap or overlap at an item boundary)", stream, i+1, d, want)
			}
		}
		if stream == "v:0" && len(deltas)+1 != 75+60+90 {
			t.Errorf("video packets %d, want 225 (2.5 + 2 + 3 s at 30 fps)", len(deltas)+1)
		}
	}
	if o, err := exec.Command("ffmpeg", "-v", "error", "-i", channel, "-f", "null", "-").CombinedOutput(); err != nil || len(o) > 0 {
		t.Fatalf("decode errors: %v\n%s", err, o)
	}
}

func ffmpegRun(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", append([]string{"-v", "error", "-y", "-nostdin"}, args...)...)
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg %v: %v\n%s", args, err, o)
	}
}

// packetDeltas returns successive DTS deltas, in the stream's time base, for one stream.
func packetDeltas(t *testing.T, path, stream string) []int {
	t.Helper()
	o, err := exec.Command("ffprobe", "-v", "error", "-select_streams", stream, "-show_entries", "packet=dts",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var out []int
	prev := -1
	for _, l := range strings.Fields(string(o)) {
		v, err := strconv.Atoi(strings.TrimSuffix(l, ","))
		if err != nil {
			t.Fatal(err)
		}
		if prev >= 0 {
			out = append(out, v-prev)
		}
		prev = v
	}
	return out
}
