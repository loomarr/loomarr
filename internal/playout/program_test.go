package playout

import (
	"math"
	"strings"
	"testing"
	"time"
)

// THE FILTER-GRAPH FAILURE the synthetic card could not find (prior-art §5b): scale emits CPU
// frames, and a hardware encoder that wants GPU frames fails with a 40-line pixel-format dump
// that never names the real problem.
//
// The invariant is NOT "every hardware encoder uploads" — an earlier version of this test
// asserted that and was simply wrong about ffmpeg. Only the families with a separate
// hardware frame pool (vaapi, qsv, vulkan) need it; nvenc, amf, videotoolbox, rkmpp and
// v4l2m2m accept CPU frames directly, and forcing an upload on them CAUSES the failure this
// test exists to prevent. The real invariant is: whoever uploads does it AFTER scaling, and
// the per-encoder decision comes from one place.
func TestScaleFilter_UploadsAfterScalingWhereRequired(t *testing.T) {
	for _, enc := range []Encoder{
		EncoderVAAPI, EncoderNVENC, EncoderQSV, EncoderVulkan,
		EncoderAMF, EncoderVideoToolbox, EncoderRKMPP, EncoderV4L2M2M,
	} {
		p := Profile{Width: 1280, Height: 720, Framerate: 25, Encoder: enc}
		vf, ok := argsAfter(p.scaleFilterArgs(""), "-vf")
		if !ok {
			t.Errorf("%s: no filter chain", enc)
			continue
		}
		// Whether this family uploads is the prober's call, not this test's — asking the same
		// helper the args use is what keeps the two from drifting.
		if hardwareUploadFilter(enc) == "" {
			if strings.Contains(vf, "hwupload") {
				t.Errorf("%s: accepts CPU frames directly; an upload here fails at init: %q", enc, vf)
			}
			continue
		}
		if !strings.Contains(vf, "hwupload") {
			t.Errorf("%s: no hwupload after scale — the encoder gets CPU frames and fails "+
				"at init: %q", enc, vf)
		}
		// Order is the whole point: uploading before scaling would run a CPU filter on GPU
		// frames, which is the same error in the other direction.
		if strings.Index(vf, "hwupload") < strings.Index(vf, "scale=") {
			t.Errorf("%s: hwupload precedes scale: %q", enc, vf)
		}
		if !strings.Contains(vf, "format=nv12") {
			t.Errorf("%s: no nv12 conversion before upload: %q", enc, vf)
		}
	}
}

// Software needs no upload — and adding one would fail, since there is no hardware device.
func TestScaleFilter_SoftwareGetsNoHardwareUpload(t *testing.T) {
	p := Profile{Width: 1280, Height: 720, Framerate: 25, Encoder: EncoderSoftware}
	vf, _ := argsAfter(p.scaleFilterArgs(""), "-vf")
	if strings.Contains(vf, "hwupload") {
		t.Errorf("software must not hwupload (there is no device): %q", vf)
	}
	// yuv420p explicitly: a 10-bit HDR source would otherwise carry its pixel format through
	// and produce a stream many players cannot decode.
	if !strings.Contains(vf, "format=yuv420p") {
		t.Errorf("no yuv420p — a 10-bit HDR source would pass its format through: %q", vf)
	}
}

// seconds must never emit exponent notation — ffmpeg would parse "1e-06" as a token rather
// than a duration.
func TestSeconds_NeverUsesExponentNotation(t *testing.T) {
	for _, d := range []time.Duration{
		time.Microsecond, time.Millisecond, time.Second,
		90*time.Minute + 500*time.Millisecond, 0,
	} {
		got := seconds(d)
		if strings.ContainsAny(got, "eE") {
			t.Errorf("seconds(%v) = %q, which ffmpeg cannot parse", d, got)
		}
	}
}

// EVERY encoder must get an explicit 8-bit pixel format, one way or another. Without it a 10-bit
// HEVC film reaches h264_nvenc as yuv420p10le and it refuses with "No capable devices found",
// which names the DEVICE, not the format, so it reads as a missing GPU.
func TestScaleFilter_EveryEncoderPinsAnEightBitPixelFormat(t *testing.T) {
	for _, enc := range encoderPreference {
		p := Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: enc}
		vf, ok := argsAfter(p.scaleFilterArgs(""), "-vf")
		if !ok {
			t.Errorf("%s: no filter chain at all", enc)
			continue
		}
		// nv12 (hardware upload path) and yuv420p (everything else) are both 8-bit. What
		// must never happen is NEITHER, which is what let 10-bit through.
		if !strings.Contains(vf, "format=nv12") && !strings.Contains(vf, "format=yuv420p") {
			t.Errorf("%s: no pixel-format pin — a 10-bit source reaches the encoder as "+
				"yuv420p10le and is rejected with a message that blames the device: %q", enc, vf)
		}
	}
}

// And specifically for the families that accept CPU frames: they must get yuv420p, since they
// have no upload filter to pin the format for them.
func TestScaleFilter_CPUFrameEncodersGetYuv420p(t *testing.T) {
	for _, enc := range []Encoder{
		EncoderNVENC, EncoderAMF, EncoderVideoToolbox, EncoderRKMPP, EncoderV4L2M2M,
	} {
		p := Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: enc}
		vf, _ := argsAfter(p.scaleFilterArgs(""), "-vf")
		if !strings.Contains(vf, "format=yuv420p") {
			t.Errorf("%s takes CPU frames and has no upload filter, so it needs an explicit "+
				"8-bit format: %q", enc, vf)
		}
		// …and must NOT have an upload, which would fail with no hardware frame context.
		if strings.Contains(vf, "hwupload") {
			t.Errorf("%s: unexpected hwupload — it accepts CPU frames directly: %q", enc, vf)
		}
	}
}

func TestStaticGainDB(t *testing.T) {
	for _, tc := range []struct {
		name             string
		target, measured float64
		want             float64
	}{
		{"already at target", -23, -23.1, 0.1},
		{"quiet clip comes up", -23, -26, 3},
		{"loud clip goes down", -23, -20, -3},
		{"absurd boost is clamped", -23, -60, MaxGainDB},
		{"absurd cut is clamped", -23, 10, -MaxGainDB},
	} {
		if got := StaticGainDB(tc.target, tc.measured); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: StaticGainDB(%v, %v) = %v, want %v", tc.name, tc.target, tc.measured, got, tc.want)
		}
	}
}

// PLACEMENT IS THE CORRECTNESS BIT, and getting it wrong produces no error — only a worse picture.
//
//   - After `fps`/`scale`, because tone-mapping is per-pixel and the scale has already cut a 4K
//     frame to 1080p. Before it, the same result costs four times the pixels on a realtime budget.
//   - Before `format=yuv420p`, because the chain's last zscale PRESERVES BIT DEPTH. Reversed, the
//     8-bit truncation happens first and the tone-map operates on an already-flattened picture —
//     which is most of what this change exists to fix.
func TestScaleFilter_TonemapsAfterScalingAndBeforePixelFormat(t *testing.T) {
	p := Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: EncoderSoftware}
	vf, ok := argsAfter(p.scaleFilterArgs(hdrToSDRChain), "-vf")
	if !ok {
		t.Fatal("no filter chain")
	}

	iScale := strings.Index(vf, "scale=1920:1080")
	iTonemap := strings.Index(vf, "tonemap=tonemap=")
	iFormat := strings.Index(vf, "format=yuv420p")
	if iScale < 0 || iTonemap < 0 || iFormat < 0 {
		t.Fatalf("chain is missing a step (scale=%d tonemap=%d format=%d): %q", iScale, iTonemap, iFormat, vf)
	}
	if iScale > iTonemap {
		t.Errorf("tone-map runs before the scale — four times the pixels for the same result: %q", vf)
	}
	if iTonemap > iFormat {
		t.Errorf("tone-map runs after the 8-bit conversion, so it maps an already-truncated "+
			"picture — the defect this fixes: %q", vf)
	}
}

// The hardware families upload to GPU memory, and the tone-map is a CPU filter — so it must land
// before the upload, or it is handed frames it cannot read.
func TestScaleFilter_TonemapsBeforeTheHardwareUpload(t *testing.T) {
	for _, enc := range []Encoder{EncoderVAAPI, EncoderQSV, EncoderVulkan} {
		p := Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: enc}
		vf, _ := argsAfter(p.scaleFilterArgs(hdrToSDRChain), "-vf")
		iTonemap, iUpload := strings.Index(vf, "tonemap=tonemap="), strings.Index(vf, "hwupload")
		if iTonemap < 0 || iUpload < 0 {
			t.Errorf("%s: chain missing tonemap or hwupload: %q", enc, vf)
			continue
		}
		if iTonemap > iUpload {
			t.Errorf("%s: CPU tone-map placed after the GPU upload; it cannot read those frames: %q", enc, vf)
		}
	}
}

// An SDR chain carries no tone-map. The overwhelming majority of programs take this path, so a
// regression here is a regression everywhere.
func TestScaleFilter_SDRChainIsUnchanged(t *testing.T) {
	p := Profile{Width: 1920, Height: 1080, Framerate: 25, Encoder: EncoderSoftware}
	vf, _ := argsAfter(p.scaleFilterArgs(""), "-vf")
	for _, unwanted := range []string{"zscale", "tonemap"} {
		if strings.Contains(vf, unwanted) {
			t.Errorf("SDR chain picked up %q: %q", unwanted, vf)
		}
	}
}
