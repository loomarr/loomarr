package hls

import (
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/media"
)

// demotedTonemappers walks one source through the packager's retry ladder (itemFaults): build,
// fail with no output, demote, until nothing is left to demote. It returns each attempt's pipeline.
func demotedTonemappers(t *testing.T, host encode.HostProfile, src media.MediaFormat, out playout.OutputProfile) []playout.Pipeline {
	t.Helper()
	faults := &itemFaults{by: map[string]itemFault{}}
	var attempts []playout.Pipeline
	for {
		p, err := playout.Build(faults.get("src").apply(host), src, out)
		if err != nil {
			t.Fatal(err)
		}
		attempts = append(attempts, p)
		if !faults.record("src", p, false) {
			return attempts
		}
	}
}

// TestDemoteTonemap_VAAPIFallsToCPU: an Intel host without the OpenCL runtime (or AMD) on a build
// without libplacebo fails device derivation before any output; the ladder then tone-maps on the CPU
// after the GPU downscale.
func TestDemoteTonemap_VAAPIFallsToCPU(t *testing.T) {
	out := playout.ChannelOutput(playout.Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: encode.EncoderVAAPI})
	attempts := demotedTonemappers(t, encode.HostFor(encode.EncoderVAAPI, true, encode.GPUFilters{TonemapOpenCL: true}), testSources()["hevc-4k-hdr-dv"], out)
	if len(attempts) != 2 || !strings.Contains(attempts[0].VideoFilter, "tonemap_opencl") {
		t.Fatalf("want OpenCL, then the end of the ladder: %d attempts, first %q", len(attempts), attempts[0].VideoFilter)
	}
	p := attempts[1]
	if !strings.Contains(p.VideoFilter, "scale_vaapi=w=1920:h=1080:force_original_aspect_ratio=decrease:force_divisible_by=2:format=p010,hwdownload,format=p010le,"+playout.HDRToSDR(media.DefaultToneCurve)) ||
		!slices.Equal(p.Fallbacks, []string{"tonemap: no GPU tone-mapper runs hable on this host"}) {
		t.Errorf("demoted VAAPI must CPU tone-map after the GPU downscale, declared: %q %q", p.VideoFilter, p.Fallbacks)
	}
}

// TestDemoteTonemap_OrderPerCurve: each family walks the curve's preferred GPU tone-mapper, the other
// one that has the curve, then the CPU. Hable is the maintainer order (#1512).
func TestDemoteTonemap_OrderPerCurve(t *testing.T) {
	all := []string{playout.TonemapperOpenCL, playout.TonemapperLibplacebo, playout.TonemapperCPU}
	noOpenCL := []string{playout.TonemapperLibplacebo, playout.TonemapperCPU}
	want := map[media.ToneCurve][]string{
		media.ToneCurveHable:    all,
		media.ToneCurveMobius:   all,
		media.ToneCurveReinhard: all,
		media.ToneCurveBT2390:   noOpenCL,
		media.ToneCurveBT2446a:  noOpenCL,
		media.ToneCurveSpline:   noOpenCL,
	}
	for _, enc := range []encode.Encoder{encode.EncoderVAAPI, encode.EncoderNVENC} {
		for _, c := range media.ToneCurves {
			out := playout.ChannelOutput(playout.Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: enc})
			out.ToneCurve = c
			var steps []string
			for _, p := range demotedTonemappers(t, encode.HostFor(enc, true, encode.GPUFilters{TonemapOpenCL: true, Libplacebo: true}), testSources()["hevc-4k-hdr-dv"], out) {
				steps = append(steps, p.Tonemapper)
			}
			if !slices.Equal(steps, want[c]) {
				t.Errorf("%s/%s: tone-map order %q, want %q", enc, c, steps, want[c])
			}
		}
	}
	sdr := demotedTonemappers(t, encode.HostFor(encode.EncoderVAAPI, true, encode.GPUFilters{TonemapOpenCL: true}), testSources()["h264-1080p-sdr-25"],
		playout.ChannelOutput(playout.Profile{Encoder: encode.EncoderVAAPI, Width: 1920, Height: 1080, Framerate: 25}))
	if len(sdr) != 1 {
		t.Error("an SDR source has no tone-mapper to demote")
	}
}
