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
// expected verdicts, e.g. "nvenc=works" for native ffmpeg n9 on a GeForce, "nvenc=disabled" for the
// image's n8.1.2 (overlay_cuda drops the picture after NVDEC), "vaapi=works" on the Arc, so the run
// cannot pass vacuously. PLAYOUT_TEST_FRAME_DIR keeps the check's encodes for inspection.
func TestLive_WatermarkCheck(t *testing.T) {
	bin := ffmpegBin(t)
	want := map[string]string{}
	for _, kv := range strings.Split(os.Getenv("PLAYOUT_TEST_WATERMARK"), ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
			want[k] = v
		}
	}
	gpu := GPUFiltersFor(bin)()
	for _, enc := range []Encoder{EncoderVAAPI, EncoderNVENC} {
		if c := trialEncodeObserved(context.Background(), bin, enc, DefaultProfile(), 1, nil); !c.Works {
			t.Logf("%s: not usable on this host (%s)", enc, firstLine(c.Err))
			if want[string(enc)] != "" {
				t.Errorf("%s: expected %q but the encoder is unusable", enc, want[string(enc)])
			}
			continue
		}
		dir := t.TempDir()
		if d := os.Getenv("PLAYOUT_TEST_FRAME_DIR"); d != "" {
			dir = d + "/" + string(enc)
		}
		host := HostFor(enc, TonemapperFor(bin)(), gpu)
		r := WatermarkCheck(context.Background(), bin, host, dir)
		verdict := map[bool]string{true: "works", false: "disabled"}[r.Works]
		t.Logf("%s: watermark %s: %s", enc, verdict, r.Detail)
		if w := want[string(enc)]; w != "" && w != verdict {
			t.Errorf("%s: watermark %s, expected %s", enc, verdict, w)
		}
	}
}
