package watermark

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Calibration on the real TMDB logos from the sample sheets (not in the repo: trademarks). Set
// WATERMARK_TEST_LOGOS to a directory holding hbo.png, nbc.png, nick.png (the splat) and nickw.png.
func TestLegibility_SampleLogos(t *testing.T) {
	dir := os.Getenv("WATERMARK_TEST_LOGOS")
	if dir == "" {
		t.Skip("WATERMARK_TEST_LOGOS not set")
	}
	want := map[string]bool{"hbo": true, "nbc": true, "nick": false, "nickw": true}
	for name, legible := range want {
		f, err := os.Open(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		mask, err := LogoMask(img)
		if err != nil {
			t.Fatal(err)
		}
		s := Legibility(mask, 1080, testLook)
		w, h := Render(mask, 1080, testLook).Size()
		t.Logf("%s: survival %.2f at %dx%d", name, s, w, h)
		if (s >= legibleSurvival) != legible {
			t.Errorf("%s: survival %.2f, sample sheet says legible=%v", name, s, legible)
		}
	}
}
