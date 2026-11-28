//go:build ffmpeg

package playout

import (
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout/media"
)

// burnBox is the test cue: a solid white 400x80 box at (760,900) on a 1920x1080 canvas, shown from
// 0.2 s to 0.8 s of the source.
var burnBox = media.Rect{X: 760, Y: 900, W: 400, H: 80}

// boxDecoder decodes every track into the box cue, as the image worker hands back a Blu-ray
// track's stills: one full-canvas PNG in the track's directory and its display window. It records
// the size and digest of the track file it was handed.
type boxDecoder struct {
	supBytes int
	sum      string
}

func (d *boxDecoder) Decode(_ context.Context, track SubtitleTrack) (SubtitleStills, error) {
	raw, err := os.ReadFile(track.Sup)
	if err != nil {
		return SubtitleStills{}, err
	}
	d.supBytes, d.sum = len(raw), track.SHA256
	img := image.NewNRGBA(image.Rect(0, 0, 1920, 1080))
	draw.Draw(img, image.Rect(burnBox.X, burnBox.Y, burnBox.X+burnBox.W, burnBox.Y+burnBox.H), image.NewUniform(color.White), image.Point{}, draw.Src)
	path := filepath.Join(track.Dir, "box.png")
	f, err := os.Create(path)
	if err != nil {
		return SubtitleStills{}, err
	}
	err = png.Encode(f, img)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return SubtitleStills{Width: 1920, Height: 1080, States: []SubtitleState{{StartMs: 200, EndMs: 800, PNG: path}}}, err
}

// supSegment is one PGS segment: its magic, presentation time (90 kHz), kind and payload.
func supSegment(at time.Duration, kind byte, data []byte) []byte {
	pts := uint32(at * 90000 / time.Second)
	h := []byte{'P', 'G', 0, 0, 0, 0, 0, 0, 0, 0, kind, 0, 0}
	binary.BigEndian.PutUint32(h[2:6], pts)
	binary.BigEndian.PutUint16(h[11:13], uint16(len(data)))
	return append(h, data...)
}

// writeSubbedSource muxes a two-display-set PGS track (a 2x2 object shown at 0.2 s, cleared at
// 0.8 s) into a short programme as its stream 1, as a Blu-ray rip carries one.
func writeSubbedSource(t *testing.T, ctx context.Context, bin, dir string) string {
	t.Helper()
	var sup []byte
	for _, set := range []struct {
		at   time.Duration
		segs [][2][]byte
	}{
		{200 * time.Millisecond, [][2][]byte{
			{{0x16}, {0x07, 0x80, 0x04, 0x38, 0x10, 0, 1, 0x80, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0}},
			{{0x17}, {1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 2}},
			{{0x14}, {0, 0, 1, 235, 128, 128, 255}},
			{{0x15}, {0, 1, 0, 0xC0, 0, 0, 8, 0, 2, 0, 2, 1, 1, 0, 0, 1, 1, 0, 0}},
			{{0x80}, nil},
		}},
		{800 * time.Millisecond, [][2][]byte{
			{{0x16}, {0x07, 0x80, 0x04, 0x38, 0x10, 0, 2, 0, 0, 0, 0}},
			{{0x17}, {1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 2}},
			{{0x80}, nil},
		}},
	} {
		for _, s := range set.segs {
			sup = append(sup, supSegment(set.at, s[0][0], s[1])...)
		}
	}
	track := filepath.Join(dir, "track.sup")
	if err := os.WriteFile(track, sup, 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "subbed.mkv")
	if b, err := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-y", "-copyts",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25:duration=1.2", "-i", track,
		"-map", "0:v", "-map", "1:s", "-c:v", "libx264", "-preset", "ultrafast", "-c:s", "copy", src).CombinedOutput(); err != nil {
		t.Fatalf("mux: %v: %s", err, b)
	}
	return src
}

// THE PREPARER COPIES THE TRACK OUT AND CONVERTS THE DECODED STILLS: the muxed stream reaches the
// decoder as a .sup, and every still is converted to an ffv1 file the list can name, the PNGs gone.
// Needs no GPU.
func TestLive_PrepareBurnIn(t *testing.T) {
	bin := ffmpegBin(t)
	ctx := context.Background()
	dir := t.TempDir()
	src := writeSubbedSource(t, ctx, bin, dir)
	var dec boxDecoder
	b, err := FFmpegBurnInWith(bin, nil, &dec)(ctx, src, 1, "hdmv_pgs_subtitle", filepath.Join(dir, "track"))
	if err != nil {
		t.Fatal(err)
	}
	if dec.supBytes == 0 || len(dec.sum) != 64 {
		t.Errorf("decoder read a %d-byte track with digest %q, want the copied stream", dec.supBytes, dec.sum)
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
	// The track is prepared from its own muxed source; the stills it gives are the box's.
	subbed := writeSubbedSource(t, p.ctx, p.bin, t.TempDir())
	track, err := FFmpegBurnInWith(p.bin, nil, &boxDecoder{})(p.ctx, subbed, 1, "hdmv_pgs_subtitle", filepath.Join(p.dir, "track"))
	if err != nil {
		t.Fatal(err)
	}
	// The item is encoded as the packager encodes it: the cue list from the item's seek, then
	// BuildItemBurning and FragmentArgs.
	encode := func(name string, seek time.Duration, subs *BurnIn) string {
		t.Helper()
		host := p.host
		var burn *SubtitleBurn
		if subs != nil {
			list, remove, err := subs.WriteList(seek)
			if err != nil {
				t.Fatal(err)
			}
			defer remove()
			burn = &SubtitleBurn{List: list, Width: subs.Width, Height: subs.Height}
			host.Overlay = true
		}
		pl, err := BuildItemBurning(host, p.facts, p.out, nil, burn)
		if err != nil {
			t.Fatal(err)
		}
		args := pl.FragmentArgs(p.src, seek, 0, 25, 47, p.out.FPS, 0)
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
	subs := track
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
		box, around := regionDiff(onY, offY, burnBox), regionDiff(onY, offY, media.Rect{X: 0, Y: 0, W: 1920, H: 800})
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

func regionMean(y []byte, r media.Rect) float64 {
	var sum float64
	for row := r.Y; row < r.Y+r.H; row++ {
		for x := r.X; x < r.X+r.W; x++ {
			sum += float64(y[row*checkWidth+x])
		}
	}
	return sum / float64(r.W*r.H)
}

func regionDiff(a, b []byte, r media.Rect) float64 {
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
