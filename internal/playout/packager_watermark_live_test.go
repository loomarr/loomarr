//go:build ffmpeg

package playout

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout/packager"
)

// A PROGRAMME WITH THE BUG AND A BREAK WITHOUT IT SPLICE AS ONE STREAM (#1512 phase 1d through the
// packager). Both items are encoded by the packager's own item command (packagerItemArgs, fMP4)
// on this host's NVENC: their SPS and PPS must be byte-identical, or the programme→break boundary
// resets the decoder. The bug-on item must keep the programme picture and blend the bug. Run it
// under the GPU lock; it skips where NVENC cannot encode. FFMPEG_PATH picks the build: n9 (native
// or the image's, since #1549) draws the bug, while n8.1.2 loses the picture under NVDEC (the
// self-check disables the watermark there, so production never airs this graph on it).
// livePackager is the packager's item command on this host's NVENC over a synthetic 1080p
// programme (testsrc2), in dir.
type livePackager struct {
	t        *testing.T
	ctx      context.Context
	bin, src string
	dir      string
	host     HostProfile
	out      OutputProfile
	facts    MediaFormat
}

// newLivePackager skips where NVENC cannot encode.
func newLivePackager(t *testing.T) *livePackager {
	bin := ffmpegBin(t)
	ctx := context.Background()
	if c := trialEncodeObserved(ctx, bin, EncoderNVENC, DefaultProfile(), 1, nil); !c.Works {
		t.Skipf("NVENC is not usable here: %s", firstLine(c.Err))
	}
	host := HostFor(EncoderNVENC, TonemapperFor(bin)(), GPUFiltersFor(bin)())
	out, _ := FormatOutput(FormatBaseline, DefaultProfile())
	out.Width, out.Height = checkWidth, checkHeight // the 1080p channel baseline the helpers decode
	dir := t.TempDir()
	if d := os.Getenv("PLAYOUT_TEST_FRAME_DIR"); d != "" {
		dir = d
	}
	src, err := synthWatermarkClip(ctx, bin, filepath.Join(dir, "programme.mkv"),
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=%d:duration=1.2", checkWidth, checkHeight, checkFPS),
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=1.2",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "25", "-c:a", "aac")
	if err != nil {
		t.Fatal(err)
	}
	return &livePackager{t: t, ctx: ctx, bin: bin, src: src, dir: dir, host: host, out: out, facts: checkClasses(host)[0].facts}
}

// encode runs one item, with bug or without (nil), and returns its H.264 elementary stream.
func (p *livePackager) encode(bug *Watermark, name string) (string, Pipeline) {
	t := p.t
	t.Helper()
	it := PackagerItem{Label: name, Input: p.src, Remaining: time.Second, Format: p.facts}
	pl, args, err := packagerItemArgs(p.host, p.out, it, bug, packager.Slot{Frames: checkFrames, AudioFrames: 47}, itemFault{})
	if err != nil {
		t.Fatal(err)
	}
	fragments := filepath.Join(p.dir, name+".mp4")
	args[len(args)-1] = fragments // pipe:1 in production
	if b, err := exec.CommandContext(p.ctx, p.bin, append([]string{"-y"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("%s: encode: %v: %s", name, err, b)
	}
	annexB := filepath.Join(p.dir, name+".h264")
	if b, err := exec.CommandContext(p.ctx, p.bin, "-hide_banner", "-loglevel", "error", "-y", "-i", fragments,
		"-map", "0:v:0", "-c", "copy", "-bsf:v", "h264_mp4toannexb", "-f", "h264", annexB).CombinedOutput(); err != nil {
		t.Fatalf("%s: remux: %v: %s", name, err, b)
	}
	return annexB, pl
}

func TestLive_PackagerWatermarkKeepsTheParameterSets(t *testing.T) {
	p := newLivePackager(t)
	ctx, bin, host, out, facts := p.ctx, p.bin, p.host, p.out, p.facts
	wm, err := writeCheckBug(p.dir)
	if err != nil {
		t.Fatal(err)
	}
	encode := p.encode
	programme, pl := encode(wm, "programme")
	if !pl.Watermark {
		t.Fatalf("the programme item carries no overlay: %v", pl.Fallbacks)
	}
	brk, _ := encode(nil, "break")
	on, err := os.ReadFile(programme)
	if err != nil {
		t.Fatal(err)
	}
	off, err := os.ReadFile(brk)
	if err != nil {
		t.Fatal(err)
	}
	if err := sameParameterSets(off, on); err != nil {
		t.Error(err)
	}
	onY, err := decodeLuma(ctx, bin, programme, checkFrame)
	if err != nil {
		t.Fatal(err)
	}
	offY, err := decodeLuma(ctx, bin, brk, checkFrame)
	if err != nil {
		t.Fatal(err)
	}
	x, y := wm.position(facts, out)
	m := compareBug(offY, onY, out.Width, Rect{X: x, Y: y, W: wm.Width, H: wm.Height})
	t.Logf("packager items on %s: picture ΔY %.2f (YAVG off %.1f, on %.1f); bug luma %.1f, want %.1f over background %.1f",
		host.Encoder, m.pictureDiff, m.pictureMean, m.onMean, m.bugLuma, m.bugWant, m.bugBackground)
	if m.pictureDiff > pictureTolerance {
		t.Errorf("the overlay lost the programme picture: mean |ΔY| %.1f outside the bug", m.pictureDiff)
	}
	if math.Abs(m.bugLuma-m.bugWant) > bugTolerance {
		t.Errorf("the bug is wrong: luma %.1f where a 65%% white blend is %.1f", m.bugLuma, m.bugWant)
	}
}
