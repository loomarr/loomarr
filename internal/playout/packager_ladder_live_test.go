//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ladderSource airs one long 4K HDR title from a fixed wall-clock start.
type ladderSource struct {
	epoch time.Time
	path  string
	facts MediaFormat
	host  HostProfile
	out   OutputProfile
}

func (s ladderSource) ItemAt(_ context.Context, _ string, at time.Time) (PackagerItem, error) {
	in := at.Sub(s.epoch)
	return PackagerItem{Label: "uhd-hdr", Input: s.path, Format: s.facts, Seek: 5*time.Second + in, Remaining: 10*time.Minute - in}, nil
}

func (ladderSource) Premium(context.Context, string) FormatClass { return "" }

func (s ladderSource) Output(context.Context, string, FormatClass, int) (HostProfile, OutputProfile) {
	return s.host, s.out
}

// stepLog records the packager's rung steps with their wall time.
type stepLog struct {
	mu    sync.Mutex
	start time.Time
	steps []string
}

func (l *stepLog) Enabled(context.Context, slog.Level) bool { return true }
func (l *stepLog) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l *stepLog) WithGroup(string) slog.Handler            { return l }
func (l *stepLog) Handle(_ context.Context, r slog.Record) error {
	if r.Message != "packager hls: software rung step" {
		return nil
	}
	var b strings.Builder
	b.WriteString(strconv.FormatFloat(r.Time.Sub(l.start).Seconds(), 'f', 1, 64) + "s")
	r.Attrs(func(a slog.Attr) bool { b.WriteString(" " + a.String()); return true })
	l.mu.Lock()
	l.steps = append(l.steps, b.String())
	l.mu.Unlock()
	return nil
}

// THE LADDER DRIVES THE LIVE PACKAGER (#1517, #1512). Run inside a CPU-limited container with no GPU
// (PLAYOUT_TEST_LADDER_4K = a 4K HDR10 source with a continuous tone; `--cpus 2`): the ledger is told
// full quality fits, so the item starts on rung 0 and the real CPU steps it down. A media-server tuner
// reads the channel throughout. The rung must step down within 30 s, every audio and video packet must
// sit exactly one frame after the last (no gap, no overlap at any splice), the tone must never go
// silent (no slate, no dropped audio), nothing may fail to decode, and the picture must be a picture.
func TestLive_PackagerLadderStepsDownWithoutAnAudioGap(t *testing.T) {
	path := os.Getenv("PLAYOUT_TEST_LADDER_4K")
	if path == "" {
		t.Skip("set PLAYOUT_TEST_LADDER_4K to a 4K HDR10 source with a continuous tone")
	}
	bin := ffmpegBin(t)
	facts, err := FFprobeFormatNextTo(bin)(context.Background(), path)
	if err != nil || !facts.HDR() || facts.Height < 2160 {
		t.Fatalf("source facts %+v (err %v): want 4K HDR", facts, err)
	}
	wall := 80 * time.Second
	if s := os.Getenv("PLAYOUT_TEST_LADDER_WALL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			wall = d
		}
	}
	dir := t.TempDir()
	steps := &stepLog{start: time.Now()}
	src := ladderSource{epoch: time.Now(), path: path, facts: facts,
		host: HostFor(EncoderSoftware, TonemapperFor(bin)(), GPUFilters{}),
		out:  ChannelOutput(Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: EncoderSoftware, AudioBitrate: 160})}
	m, err := NewPackagerHLS(src, bin, filepath.Join(dir, "hls"), time.Second, slog.New(steps))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	// The ledger believes full quality fits (an optimistic measurement), so only the live ladder can
	// bring the item down to what this CPU sustains.
	m.WithBudget(NewResourceBudget(func() BudgetFacts {
		return BudgetFacts{CPUAllowance: 8, Rungs: []int{1080}, Costs: map[CostKey]ClassCost{
			{Class: ClassSDR, Height: 1080}:   {Speed: 3, CPUCores: 1.4},
			{Class: ClassHDR4K, Height: 1080}: {Speed: 0.5, CPUCores: 3.6},
		}}
	}))
	o := NewOrigin(OriginDependencies{Packager: m})
	ctx, cancel := context.WithTimeout(context.Background(), wall)
	defer cancel()
	tuner, err := o.Tune(ctx, TuneRequest{ChannelID: "ch", Plan: PlanBaseline, Delivery: DeliveryMPEGTS})
	if err != nil {
		t.Fatal(err)
	}
	defer tuner.Release()
	var ts bytes.Buffer
	for {
		b, err := tuner.Stream.Next(ctx)
		if err != nil {
			break // the wall-clock budget ended the read
		}
		ts.Write(b)
	}
	m.mu.Lock()
	c := m.channels[packagedKey{channel: "ch", format: FormatBaseline}]
	m.mu.Unlock()
	stats := c.p.Stats()
	steps.mu.Lock()
	t.Logf("LADDER steps: %q", steps.steps)
	first := ""
	if len(steps.steps) > 0 {
		first = steps.steps[0]
	}
	steps.mu.Unlock()
	out := filepath.Join(dir, "tuner.ts")
	if err := os.WriteFile(out, ts.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("LADDER packager stats %+v; tuner read %d bytes, %.1f s of video", stats, ts.Len(), tsDuration(t, dir, ts.Bytes()).Seconds())

	if first == "" {
		t.Fatal("the rung never stepped down")
	}
	if at, _ := strconv.ParseFloat(first[:strings.IndexByte(first, 's')], 64); at > 30 {
		t.Errorf("first step at %.1f s after the tune; want within 30 s", at)
	}
	if !strings.Contains(first, "rung=rung1-light") {
		t.Errorf("first step %q: want full to rung 1", first)
	}
	for stream, want := range map[string]int{"v:0": 3600, "a:0": 1920} { // 90 kHz: 1/25 s, 1024/48000 s
		for i, d := range packetDeltas(t, out, stream) {
			if d != want {
				t.Fatalf("%s packet %d delta %d, want %d (a gap or overlap at a splice)", stream, i+1, d, want)
			}
		}
	}
	if o, err := exec.Command(bin, "-v", "error", "-i", out, "-f", "null", "-").CombinedOutput(); err != nil || len(o) > 0 {
		t.Fatalf("decode errors: %v\n%s", err, o)
	}
	sil, _ := exec.Command(bin, "-hide_banner", "-nostats", "-i", out, "-map", "0:a:0",
		"-af", "silencedetect=n=-40dB:d=0.05", "-f", "null", "-").CombinedOutput()
	if n := strings.Count(string(sil), "silence_start"); n > 0 {
		t.Errorf("the continuous tone went silent %d time(s) (a slate or a dropped splice):\n%s", n, grepLines(string(sil), "silence_"))
	}
	if stats.Slates > 0 {
		t.Errorf("the channel slated %d time(s); a resumed rung must hold, not slate", stats.Slates)
	}
	assertPicture(t, bin, out)
}

func grepLines(s, sub string) string {
	var b strings.Builder
	for line := range strings.Lines(s) {
		if strings.Contains(line, sub) {
			b.WriteString(line)
		}
	}
	return b.String()
}
