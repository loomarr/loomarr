//go:build ffmpeg

package playout

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout/packager"
)

// burnBox is the test cue: a solid white 400x80 box at (760,900) on a 1920x1080 canvas, shown from
// 0.2 s to 0.8 s of the source.
var burnBox = Rect{X: 760, Y: 900, W: 400, H: 80}

// writeBurnSource muxes a .sup carrying the box into a short programme, as a Blu-ray rip carries it.
func writeBurnSource(t *testing.T, ctx context.Context, bin, dir string) string {
	t.Helper()
	var row []byte
	row = append(row, 0, 0xC0|byte(burnBox.W>>8), byte(burnBox.W&0xFF), 1, 0, 0)
	var rle []byte
	for range burnBox.H {
		rle = append(rle, row...)
	}
	var w supWriter
	w.show(200*time.Millisecond, pgsEpochStart, burnBox.X, burnBox.Y, rle, burnBox.W, burnBox.H)
	w.clear(800 * time.Millisecond)
	sup := filepath.Join(dir, "track.sup")
	if err := os.WriteFile(sup, w.b, 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "subbed.mkv")
	// -copyts: the .sup is its own input, whose start (its first cue) would otherwise become zero.
	if b, err := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-y", "-copyts",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=25:duration=1.2", "-i", sup,
		"-map", "0:v", "-map", "1:s", "-c:v", "libx264", "-preset", "ultrafast", "-c:s", "copy", src).CombinedOutput(); err != nil {
		t.Fatalf("mux: %v: %s", err, b)
	}
	return src
}

// THE PREPARER READS A REAL MUXED TRACK: copied out as .sup, decoded, and every still converted
// to an ffv1 file the list can name. Needs no GPU.
func TestLive_PrepareBurnIn(t *testing.T) {
	bin := ffmpegBin(t)
	ctx := context.Background()
	dir := t.TempDir()
	src := writeBurnSource(t, ctx, bin, dir)
	b, err := FFmpegBurnInWith(bin, nil)(ctx, src, 0, filepath.Join(dir, "track"))
	if err != nil {
		t.Fatal(err)
	}
	if b.Width != 1920 || b.Height != 1080 || len(b.Cues) != 2 ||
		b.Cues[0].At != 200*time.Millisecond || b.Cues[1].Still != burnInBlank {
		t.Fatalf("prepared %+v", b)
	}
	for _, f := range []string{b.Cues[0].Still, burnInBlank, burnInIndex} {
		if _, err := os.Stat(filepath.Join(b.Dir, f)); err != nil {
			t.Error(err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(b.Dir, "*.png")); len(left) > 0 {
		t.Errorf("PNGs left behind: %v", left)
	}
	if again, ok := LoadBurnIn(b.Dir); !ok || len(again.Cues) != 2 {
		t.Errorf("reload: %+v %v", again, ok)
	}
}

// THE PACKAGER BURNS THE TRACK IN ON THE GPU, ON TIME FROM ANY SEEK. The same item is encoded from
// its start and from 0.5 s (a tune-in mid-cue, the case a -ss seek of the stills loses): the box
// must be white while its cue shows, gone after, and the picture around it untouched. Runs on
// NVENC under the GPU lock; it skips where NVENC cannot encode.
func TestLive_PackagerBurnsSubtitles(t *testing.T) {
	p := newLivePackager(t)
	track, err := FFmpegBurnInWith(p.bin, nil)(p.ctx, writeBurnSource(t, p.ctx, p.bin, p.dir), 0, filepath.Join(p.dir, "track"))
	if err != nil {
		t.Fatal(err)
	}
	encode := func(name string, seek time.Duration, subs *ItemSubtitles) string {
		t.Helper()
		it := PackagerItem{Label: name, Input: p.src, Seek: seek, Remaining: time.Second, Format: p.facts}
		burn, remove := itemBurn(it, subs, slog.Default())
		defer remove()
		pl, args, err := packagerItemArgs(p.host, p.out, it, nil, burn, packager.Slot{Frames: 25, AudioFrames: 47}, itemFault{})
		if err != nil {
			t.Fatal(err)
		}
		if subs != nil && !pl.Subtitles {
			t.Fatalf("%s: no subtitles: %v", name, pl.Fallbacks)
		}
		out := filepath.Join(p.dir, name+".mp4")
		args[len(args)-1] = out
		if b, err := exec.CommandContext(p.ctx, p.bin, append([]string{"-y"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%s: encode: %v: %s", name, err, b)
		}
		return out
	}
	subs := &ItemSubtitles{Track: track}
	for _, c := range []struct {
		name  string
		seek  time.Duration
		frame int
		shows bool
	}{
		{"from-start-before", 0, 2, false},                   // 0.08 s
		{"from-start-during", 0, 12, true},                   // 0.48 s
		{"from-start-after", 0, 22, false},                   // 0.88 s
		{"mid-cue-during", 500 * time.Millisecond, 2, true},  // 0.58 s
		{"mid-cue-after", 500 * time.Millisecond, 10, false}, // 0.90 s
	} {
		on := encode(c.name, c.seek, subs)
		off := encode(c.name+"-off", c.seek, nil)
		onY, err := decodeLuma(p.ctx, p.bin, on, c.frame)
		if err != nil {
			t.Fatal(err)
		}
		offY, err := decodeLuma(p.ctx, p.bin, off, c.frame)
		if err != nil {
			t.Fatal(err)
		}
		box, around := regionDiff(onY, offY, burnBox), regionDiff(onY, offY, Rect{X: 0, Y: 0, W: 1920, H: 800})
		boxY := regionMean(onY, burnBox)
		t.Logf("%s: box Y %.1f (|ΔY| %.1f), picture |ΔY| %.2f", c.name, boxY, box, around)
		if around > pictureTolerance {
			t.Errorf("%s: the burn changed the picture: |ΔY| %.1f", c.name, around)
		}
		if c.shows && boxY < 225 {
			t.Errorf("%s: the cue is not burned in: box Y %.1f, want white", c.name, boxY)
		}
		if !c.shows && box > pictureTolerance {
			t.Errorf("%s: a cue shows when none should: |ΔY| %.1f", c.name, box)
		}
	}
}

func regionMean(y []byte, r Rect) float64 {
	var sum float64
	for row := r.Y; row < r.Y+r.H; row++ {
		for x := r.X; x < r.X+r.W; x++ {
			sum += float64(y[row*checkWidth+x])
		}
	}
	return sum / float64(r.W*r.H)
}

func regionDiff(a, b []byte, r Rect) float64 {
	var sum float64
	for row := r.Y; row < r.Y+r.H; row++ {
		off := row * checkWidth
		sum += float64(absDiffSum(a[off+r.X:off+r.X+r.W], b[off+r.X:off+r.X+r.W]))
	}
	return sum / float64(r.W*r.H)
}

func absDiffSum(a, b []byte) int {
	n := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		n += d
	}
	return n
}
