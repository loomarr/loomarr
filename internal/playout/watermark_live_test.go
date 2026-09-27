//go:build ffmpeg

package playout

import (
	"context"
	"os"
	"strings"
	"testing"
)

// THE WATERMARK SELF-CHECK ON THIS HOST'S REAL GPU (#1512 phase 1d). It runs WatermarkCheck for each
// GPU family this ffmpeg can encode with and logs the verdict. Set PLAYOUT_TEST_WATERMARK to the
// expected verdicts by FAMILY, e.g. "nvenc=works" for native ffmpeg n9 on a GeForce,
// "nvenc=disabled" for the image's n8.1.2 (overlay_cuda drops the picture after NVDEC),
// "vaapi=works" on the Arc, so the run cannot pass vacuously: a verdict that differs, a family
// that cannot encode here, or an unknown key or verdict fails. PLAYOUT_TEST_FRAME_DIR keeps the
// check's encodes for inspection.
func TestLive_WatermarkCheck(t *testing.T) {
	bin := ffmpegBin(t)
	want := parseWatermarkWant(t, os.Getenv("PLAYOUT_TEST_WATERMARK"))
	gpu := GPUFiltersFor(bin)()
	for _, enc := range []Encoder{EncoderVAAPI, EncoderNVENC} {
		host := HostFor(enc, TonemapperFor(bin)(), gpu)
		w := want[host.Family]
		if c := trialEncodeObserved(context.Background(), bin, enc, DefaultProfile(), 1, nil); !c.Works {
			t.Logf("%s: not usable on this host (%s)", host.Family, firstLine(c.Err))
			if w != "" {
				t.Errorf("%s: expected watermark %s but the encoder is unusable", host.Family, w)
			}
			continue
		}
		dir := t.TempDir()
		if d := os.Getenv("PLAYOUT_TEST_FRAME_DIR"); d != "" {
			dir = d + "/" + string(host.Family)
		}
		r := WatermarkCheck(context.Background(), bin, host, dir)
		verdict := map[bool]string{true: "works", false: "disabled"}[r.Works]
		t.Logf("%s: watermark %s: %s", host.Family, verdict, r.Detail)
		if w != "" && w != verdict {
			t.Errorf("%s: watermark %s, expected %s", host.Family, verdict, w)
		}
	}
}

// parseWatermarkWant reads "family=verdict,..." and refuses what the test could never match: a
// key that named the encoder ("h264_vaapi") instead of the family once made every expectation a
// silent no-op on the Arc.
func parseWatermarkWant(t *testing.T, s string) map[Family]string {
	t.Helper()
	want := map[Family]string{}
	for _, kv := range strings.Split(s, ",") {
		if strings.TrimSpace(kv) == "" {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		switch {
		case !ok:
			t.Fatalf("PLAYOUT_TEST_WATERMARK: %q is not family=verdict", kv)
		case Family(k) != FamilyVAAPI && Family(k) != FamilyNVENC:
			t.Fatalf("PLAYOUT_TEST_WATERMARK: unknown family %q (want %s or %s)", k, FamilyVAAPI, FamilyNVENC)
		case v != "works" && v != "disabled":
			t.Fatalf("PLAYOUT_TEST_WATERMARK: unknown verdict %q for %s (want works or disabled)", v, k)
		}
		want[Family(k)] = v
	}
	return want
}
