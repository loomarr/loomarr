//go:build ffmpeg

package mediatools_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/loomarr/loomarr/internal/defects"
	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/testkit"
)

// Each fixture is damaged one way and judged as an HD-era ad, the strictest bar: it fails its own
// check, and the clean clip passes every check.
func TestMeasureDefects_EachDamageFailsItsOwnCheck(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("ffmpeg build-tag test requires ffmpeg")
	}
	fx := testkit.FillerDefectMedia(t, t.TempDir())
	ctx := context.Background()
	measure := func(file string) defects.Readings {
		t.Helper()
		r, err := mediatools.MeasureDefects(ctx, ffmpeg, file, 640, 480, true)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	outcomes := func(r defects.Readings) map[defects.Check]defects.Outcome {
		out := map[defects.Check]defects.Outcome{}
		for _, f := range defects.Assess(r, 2015).Findings {
			out[f.Check] = f.Outcome
		}
		return out
	}

	clean := measure(fx.Clean)
	if a := defects.Assess(clean, 2015); a.Outcome != defects.Pass {
		t.Fatalf("clean clip = %+v from %+v, want a pass", a, clean)
	}
	for name, c := range map[string]struct {
		file  string
		check defects.Check
	}{
		"blocky":      {fx.Blocky, defects.CheckBlocks},
		"blurred":     {fx.Blurred, defects.CheckBlur},
		"upscaled":    {fx.Upscaled, defects.CheckUpscale},
		"windowboxed": {fx.Windowboxed, defects.CheckWindowbox},
		"combed":      {fx.Combed, defects.CheckInterlace},
		"dropouts":    {fx.Dropouts, defects.CheckDropouts},
		"clipped":     {fx.Clipped, defects.CheckClipping},
	} {
		t.Run(name, func(t *testing.T) {
			r := measure(c.file)
			if got := outcomes(r)[c.check]; got != defects.Fail {
				t.Fatalf("%s: %s = %q, want fail (readings %+v)", name, c.check, got, r)
			}
		})
	}
}
