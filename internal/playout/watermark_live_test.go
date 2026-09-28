//go:build ffmpeg

package playout

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// decodeLuma decodes frame n of an H.264 elementary stream to its CODED 8-bit luma: the Y plane of
// yuv420p, in the stream's own limited range. Never -pix_fmt gray: swscale converts to full-range
// grey rather than copying Y (a neutral Y 171 reads 180; the check's coloured fixture reads 163
// where Y is 170), so the expectation, a 65% blend of limited-range 235, was in the wrong space
// (#1541).
func decodeLuma(ctx context.Context, ffmpeg, path string, n int) ([]byte, error) {
	yuv, err := decodeFrame(ctx, ffmpeg, path, n)
	if err != nil {
		return nil, err
	}
	return yuv[:checkWidth*checkHeight], nil
}

// THE WATERMARK SELF-CHECK ON THIS HOST'S REAL GPU (#1512 phase 1d). It runs WatermarkCheck for each
// GPU family this ffmpeg can encode with and logs the verdict. Set PLAYOUT_TEST_WATERMARK to the
// expected verdicts by FAMILY, e.g. "nvenc=works" for ffmpeg n9 (native or the image's) on a GeForce,
// "nvenc=disabled" for n8.1.2 (overlay_cuda drops the picture after NVDEC),
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

// The self-check measures CODED luma (#1541): its expectation is a 65% blend of limited-range white
// 235, so the decode must return Y as coded. -pix_fmt gray is a full-range grey conversion instead
// (a neutral Y 171 reads 180), which put the expectation in the wrong space.
func TestDecodeLuma_ReadsCodedLimitedRangeLuma(t *testing.T) {
	bin := ffmpegBin(t)
	path := t.TempDir() + "/grey.h264"
	// RGB 180 grey in BT.709 limited range is Y 16+219·180/255 = 170.6.
	if out, err := exec.Command(bin, "-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i",
		fmt.Sprintf("color=c=0xB4B4B4:s=%dx%d:r=%d:d=1", checkWidth, checkHeight, checkFPS),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-f", "h264", path).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, out)
	}
	y, err := decodeLuma(context.Background(), bin, path, checkFrame)
	if err != nil {
		t.Fatal(err)
	}
	if got := y[len(y)/2]; got < 170 || got > 172 {
		t.Fatalf("luma %d, want the coded 171 (180 is the full-range gray reading)", got)
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
