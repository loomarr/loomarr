package tools_test

import (
	"context"
	"image/color"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/media/tools"
)

// The frame-heuristic tier (§10 V44) is deterministic and LLM-free, so it is
// tested on SYNTHETIC raw rgb24 frames built here — no ffmpeg, no network (§19).
// TestFrameHintsIn_MeasuresRawFramesFromFFmpeg covers the ffmpeg half, with the
// real decoder's chroma artefacts, against bwChromaThreshold.

// solidFrame builds a w×h rgb24 frame filled with one colour. A grey fill (r==g==b)
// is the monochrome case; anything else is a colour frame.
func solidFrame(t *testing.T, w, h int, c color.RGBA) tools.RawFrame {
	t.Helper()
	pix := make([]byte, 0, 3*w*h)
	for range w * h {
		pix = append(pix, c.R, c.G, c.B)
	}
	return tools.RawFrame{Width: w, Height: h, RGB: pix}
}

// grey/colour/dimension helpers, so the table rows read as intent not arithmetic.
func gray(v uint8) color.RGBA { return color.RGBA{v, v, v, 255} }
func red() color.RGBA         { return color.RGBA{200, 30, 30, 255} }

const fourThreeW, fourThreeH = 128, 96     // 1.333…
const sixteenNineW, sixteenNineH = 128, 72 // 1.777…

func TestAnalyzeFrames_BlackAndWhiteDetection(t *testing.T) {
	tests := []struct {
		name   string
		frames []tools.RawFrame
		wantBW bool
		wantN  int
	}{
		{
			name:   "all grey frames read monochrome",
			frames: []tools.RawFrame{grayF(t, 40), grayF(t, 128), grayF(t, 210)},
			wantBW: true,
			wantN:  3,
		},
		{
			name:   "all colour frames do not",
			frames: []tools.RawFrame{colourF(t), colourF(t), colourF(t)},
			wantBW: false,
			wantN:  3,
		},
		{
			// A single grey fade in a colour spot must NOT make the clip B&W — the
			// majority rule (bwFrameFraction) exists for exactly this.
			name:   "one grey frame among colour is not enough",
			frames: []tools.RawFrame{grayF(t, 128), colourF(t), colourF(t)},
			wantBW: false,
			wantN:  3,
		},
		{
			// Chroma noise on a B&W transfer: a mean spread of 6, the most a heavily
			// noised B&W clip measures at FrameHintWidth, stays monochrome.
			name:   "noisy grey frames read monochrome",
			frames: []tools.RawFrame{spreadF(t, 6), spreadF(t, 6), spreadF(t, 6)},
			wantBW: true,
			wantN:  3,
		},
		{
			// A faint, noised colour clip: a mean spread of 11, just under the least
			// such a clip measures at FrameHintWidth, is colour.
			name:   "faint colour frames do not",
			frames: []tools.RawFrame{spreadF(t, 11), spreadF(t, 11), spreadF(t, 11)},
			wantBW: false,
			wantN:  3,
		},
		{
			// The mirror: a majority grey DOES qualify even with a stray colour frame.
			name:   "majority grey with one colour frame qualifies",
			frames: []tools.RawFrame{grayF(t, 80), grayF(t, 160), colourF(t)},
			wantBW: true,
			wantN:  3,
		},
		{
			// A truncated frame is skipped, not fatal, and does not count toward N.
			name:   "undecodable frames are ignored",
			frames: []tools.RawFrame{grayF(t, 100), {Width: fourThreeW, Height: fourThreeH, RGB: []byte("short")}, grayF(t, 150)},
			wantBW: true,
			wantN:  2,
		},
		{
			name:   "no frames yields no measurement",
			frames: nil,
			wantBW: false,
			wantN:  0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tools.AnalyzeFrames(tc.frames)
			if got.BlackAndWhite != tc.wantBW {
				t.Errorf("BlackAndWhite = %v, want %v", got.BlackAndWhite, tc.wantBW)
			}
			if got.Frames != tc.wantN {
				t.Errorf("Frames = %d, want %d", got.Frames, tc.wantN)
			}
		})
	}
}

func TestAnalyzeFrames_AspectRatio(t *testing.T) {
	tests := []struct {
		name   string
		frames []tools.RawFrame
		wantLo float64 // aspect ratio band [wantLo, wantHi]
		wantHi float64
	}{
		{
			name:   "4:3 frames average ~1.333",
			frames: []tools.RawFrame{solidFrame(t, fourThreeW, fourThreeH, gray(128))},
			wantLo: 1.30, wantHi: 1.36,
		},
		{
			name:   "16:9 frames average ~1.778",
			frames: []tools.RawFrame{solidFrame(t, sixteenNineW, sixteenNineH, gray(128))},
			wantLo: 1.74, wantHi: 1.82,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tools.AnalyzeFrames(tc.frames)
			if got.AspectRatio < tc.wantLo || got.AspectRatio > tc.wantHi {
				t.Errorf("AspectRatio = %.4f, want in [%.2f,%.2f]", got.AspectRatio, tc.wantLo, tc.wantHi)
			}
		})
	}
}

// The hint is the whole point: a B&W 4:3 clip SUGGESTS a pre-1970s era, and every
// other combination suggests nothing. The suggestion is never a grounded Era — the
// caller records it as an uncited era observation, which matching never reads.
func TestSuggestedEraFrom(t *testing.T) {
	bw43 := tools.AnalyzeFrames([]tools.RawFrame{
		solidFrame(t, fourThreeW, fourThreeH, gray(60)),
		solidFrame(t, fourThreeW, fourThreeH, gray(140)),
	})
	colour43 := tools.AnalyzeFrames([]tools.RawFrame{
		solidFrame(t, fourThreeW, fourThreeH, red()),
		solidFrame(t, fourThreeW, fourThreeH, red()),
	})
	bw169 := tools.AnalyzeFrames([]tools.RawFrame{
		solidFrame(t, sixteenNineW, sixteenNineH, gray(60)),
		solidFrame(t, sixteenNineW, sixteenNineH, gray(140)),
	})

	if got := tools.SuggestedEraFrom(bw43); got == 0 {
		t.Errorf("B&W 4:3 should suggest an era, got 0")
	}
	// ⚠ The value is a DECADE sentinel (a hint), never a specific fabricated year.
	if got := tools.SuggestedEraFrom(bw43); got%10 != 0 || got >= 1970 {
		t.Errorf("B&W 4:3 hint = %d, want a pre-1970s decade sentinel", got)
	}
	// Colour, or widescreen, or nothing: no hint. Neither signal alone dates a clip.
	if got := tools.SuggestedEraFrom(colour43); got != 0 {
		t.Errorf("colour 4:3 should suggest nothing, got %d", got)
	}
	if got := tools.SuggestedEraFrom(bw169); got != 0 {
		t.Errorf("B&W 16:9 should suggest nothing, got %d", got)
	}
	if got := tools.SuggestedEraFrom(tools.FrameHint{}); got != 0 {
		t.Errorf("no frames should suggest nothing, got %d", got)
	}
}

// A hint is only ever a SUGGESTION: SuggestedEraFrom must never return a value the
// grounding rules would accept as a fact. It is a decade sentinel a human confirms,
// exactly as validateTags demotes an ungrounded LLM year to SuggestedEra.
func TestSuggestedEraFrom_IsSuggestionNotGroundedTag(t *testing.T) {
	bw43 := tools.AnalyzeFrames([]tools.RawFrame{
		solidFrame(t, fourThreeW, fourThreeH, gray(70)),
		solidFrame(t, fourThreeW, fourThreeH, gray(150)),
	})
	hint := tools.SuggestedEraFrom(bw43)
	if hint == 0 {
		t.Fatalf("expected a suggestion for a B&W 4:3 clip")
	}
}

func grayF(t *testing.T, v uint8) tools.RawFrame {
	return solidFrame(t, fourThreeW, fourThreeH, gray(v))
}
func colourF(t *testing.T) tools.RawFrame { return solidFrame(t, fourThreeW, fourThreeH, red()) }

// spreadF builds a 4:3 frame of mid-grey pixels whose channels alternate by ±spread/2
// around 128, so every pixel's max(R,G,B)-min(R,G,B) — and the frame's mean — is
// spread. Alternating which channel is high mimics noise rather than a steady tint.
func spreadF(t *testing.T, spread uint8) tools.RawFrame {
	t.Helper()
	lo, hi := 128-spread/2, 128-spread/2+spread
	pix := make([]byte, 0, 3*fourThreeW*fourThreeH)
	for i := range fourThreeW * fourThreeH {
		if i%2 == 0 {
			pix = append(pix, hi, 128, lo)
		} else {
			pix = append(pix, lo, 128, hi)
		}
	}
	return tools.RawFrame{Width: fourThreeW, Height: fourThreeH, RGB: pix}
}

// The ffmpeg half: FrameHintsIn decodes real encoded video into raw frames and measures them,
// so the codec's chroma artefacts meet bwChromaThreshold here. A monochrome 4:3 clip hints
// pre-1970s; a colour 16:9 clip measures widescreen colour and hints nothing.
func TestFrameHintsIn_MeasuresRawFramesFromFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	dir := t.TempDir()
	clip := func(name, size, vf string) string {
		path := filepath.Join(dir, name)
		args := []string{"-nostdin", "-v", "error", "-f", "lavfi",
			"-i", "testsrc2=size=" + size + ":rate=10:duration=6"}
		if vf != "" {
			args = append(args, "-vf", vf)
		}
		args = append(args, "-c:v", "mpeg4", "-pix_fmt", "yuv420p", "-y", path)
		if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
			t.Fatalf("build fixture %s: %v: %s", name, err, out)
		}
		return path
	}
	ft := tools.NewFFmpegTools(ffmpeg, "", "", "", "")

	bw, err := ft.FrameHintsIn(context.Background(), clip("bw.mp4", "640x480", "hue=s=0"), 0, 6000, 3)
	if err != nil {
		t.Fatal(err)
	}
	if bw.Frames != 3 || !bw.BlackAndWhite || bw.AspectRatio < 1.30 || bw.AspectRatio > 1.36 {
		t.Fatalf("B&W 4:3 hint = %+v, want 3 monochrome frames at ~1.333", bw)
	}
	if tools.SuggestedEraFrom(bw) == 0 {
		t.Errorf("B&W 4:3 clip suggests no era")
	}

	colour, err := ft.FrameHintsIn(context.Background(), clip("colour.mp4", "1280x720", ""), 0, 6000, 3)
	if err != nil {
		t.Fatal(err)
	}
	if colour.Frames != 3 || colour.BlackAndWhite || colour.AspectRatio < 1.74 || colour.AspectRatio > 1.82 {
		t.Fatalf("colour 16:9 hint = %+v, want 3 colour frames at ~1.778", colour)
	}
}
