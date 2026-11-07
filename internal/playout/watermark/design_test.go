package watermark

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var updateLooks = flag.Bool("update-looks", false, "rewrite testdata/looks.golden")

// designCase is one golden render: every look × {white, coloured} × {no motif, each motif} at the
// 1080p and 480p bug sizes (#1659 acceptance).
type designCase struct {
	name   string
	design Design
	height int
}

func designCases() []designCase {
	var cases []designCase
	for _, s := range DesignStyles {
		for _, white := range []bool{false, true} {
			for _, m := range append([]Motif{""}, Motifs...) {
				for _, h := range []int{1080, 480} {
					d := Design{Style: s, Callsign: "SC", Number: 7, Colour: Red, Palette: EraPalette(1980), Motif: m, White: white}
					tone := "colour"
					if white {
						tone = "white"
					}
					cases = append(cases, designCase{fmt.Sprintf("%s-%s-%s-%d", s, tone, cmpOr(string(m), "none"), h), d, h})
				}
			}
		}
	}
	return cases
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// TestRenderDesign_Golden pins every render's pixels by digest (testdata/looks.golden; rewrite with
// -update-looks after an intended change, and bump renderVersion in internal/app). Set
// WATERMARK_DESIGN_DIR to also write each PNG for review.
func TestRenderDesign_Golden(t *testing.T) {
	dir := os.Getenv("WATERMARK_DESIGN_DIR")
	cases := designCases()
	got := make([]string, len(cases))
	// The renders are independent, so they run in parallel, each into its own line. One at a time
	// they were this package's whole time under -race, on one of a CI lane's four cores, and the
	// lane's longest pole (#2007).
	t.Run("render", func(t *testing.T) {
		for i, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				bug, err := RenderDesign(c.design, c.height, testLook)
				if err != nil {
					t.Fatal(err)
				}
				w, h := bug.Size()
				if w%2 != 0 || h%2 != 0 {
					t.Errorf("odd size %dx%d", w, h)
				}
				sum := sha256.Sum256(append(bug.Straight.Pix, byte(w), byte(w>>8), byte(h), byte(h>>8)))
				got[i] = fmt.Sprintf("%s %dx%d %s", c.name, w, h, hex.EncodeToString(sum[:12]))
				if dir != "" {
					writePNG(t, filepath.Join(dir, c.name+".png"), bug.Straight)
				}
			})
		}
	})
	if t.Failed() {
		return
	}
	path := filepath.Join("testdata", "looks.golden")
	text := strings.Join(got, "\n") + "\n"
	if *updateLooks {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden (run with -update-looks): %v", err)
	}
	wantLines := strings.Split(strings.TrimSpace(string(want)), "\n")
	for i, line := range got {
		if i >= len(wantLines) || line != wantLines[i] {
			t.Errorf("render drifted: got %q, want %q", line, slices.Concat(wantLines, []string{""})[min(i, len(wantLines))])
		}
	}
	if len(wantLines) != len(got) {
		t.Errorf("%d golden lines, %d renders", len(wantLines), len(got))
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// A built look with no motif is the built renderer's bug, byte for byte: existing channels' bugs
// do not change with the new renderer.
func TestRenderDesign_BuiltLookUnchanged(t *testing.T) {
	for _, s := range Styles {
		mask, err := CallsignMask("RETRO", s)
		if err != nil {
			t.Fatal(err)
		}
		want := Render(mask, 1080, s.Adjust(testLook))
		got, err := RenderDesign(Design{Style: s, Callsign: "RETRO", Colour: Red, Palette: NoEraPalette}, 1080, testLook)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got.Straight.Pix, want.Straight.Pix) {
			t.Errorf("%s: the design renderer changed a built look", s)
		}
	}
}

// White draws no colour: every visible pixel of a white design is neutral (R = G = B), shadow
// included, and a coloured design is not.
func TestRenderDesign_WhiteIsNeutral(t *testing.T) {
	for _, white := range []bool{true, false} {
		bug, err := RenderDesign(Design{Style: StyleTint, Callsign: "SC", Number: 7, Colour: Red, Palette: NoEraPalette, Motif: MotifPumpkin, White: white}, 1080, testLook)
		if err != nil {
			t.Fatal(err)
		}
		chroma := false
		p := bug.Straight.Pix
		for i := 0; i < len(p); i += 4 {
			if p[i+3] > 8 && (p[i] != p[i+1] || p[i+1] != p[i+2]) {
				chroma = true
				break
			}
		}
		if chroma == white {
			t.Errorf("white=%v: coloured pixels present=%v", white, chroma)
		}
	}
}

func TestEraPalette_UndefinedDecadeIsNoEra(t *testing.T) {
	if EraPalette(1980) != (Palette{Red, Amber, Cyan}) || EraPalette(1960) != NoEraPalette || EraPalette(0) != NoEraPalette {
		t.Error("era palette table drifted from the approved mock")
	}
}

func TestRenderDesign_NumberLooksWithoutANumberDrawTheCallsign(t *testing.T) {
	got, err := RenderDesign(Design{Style: StyleBadge, Callsign: "SC", Colour: Red}, 1080, testLook)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := RenderDesign(Design{Style: StyleTint, Callsign: "SC", Colour: Red}, 1080, testLook)
	if !slices.Equal(got.Straight.Pix, want.Straight.Pix) {
		t.Error("a badge with no channel number should draw the Colour plate")
	}
}

// The picture-only chip draws each motif with the bug's own colours (white when asked), as a
// square of the asked size, and refuses an unknown motif.
func TestRenderMotif(t *testing.T) {
	for _, m := range Motifs {
		img, err := RenderMotif(m, false, 48)
		if err != nil || img.Rect.Dx() != 48 || img.Rect.Dy() != 48 {
			t.Fatalf("%s: %v %v", m, img.Rect, err)
		}
		if visibleBox(img).Empty() {
			t.Errorf("%s drew nothing", m)
		}
	}
	white, _ := RenderMotif(MotifPumpkin, true, 48)
	for i := 0; i < len(white.Pix); i += 4 {
		// Neutral, not exactly 255: resampling rounds the faint edge pixels.
		if r, g, b := white.Pix[i], white.Pix[i+1], white.Pix[i+2]; white.Pix[i+3] > 32 && (r != g || g != b) {
			t.Fatalf("white pumpkin has a coloured pixel %v", white.Pix[i:i+4])
		}
	}
	if _, err := RenderMotif("arbor", false, 48); err == nil {
		t.Error("an unknown motif drew")
	}
}
