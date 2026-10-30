package encode

import (
	"slices"
	"strings"
	"testing"
)

// AN HEVC ENCODER MUST GET ITS ENGINE'S HARDWARE PLUMBING (§9.1 V49).
//
// DeviceInitArgs, UploadFilter and DecodeArgs each switch on the encoder. All three once listed
// only the h264 constants, so every hevc_* encoder fell through to `default` and received nothing:
// no `-vaapi_device`, no `-init_hw_device` for QSV/Vulkan, no `-hwaccel cuda` for NVENC.
//
// The consequence was worse than a clean failure. The hardware encode produced no bytes, the
// program ladder fell back to libx264, and for an fMP4 (HEVC) session that mid-stream codec change
// is the black frame the plan exists to prevent — so a defect in the DEVICE SETUP surfaced as a
// codec bug three layers away.
//
// The assertion is pairwise rather than a table of expected strings: an engine's h264 and HEVC
// encoders run on the SAME hardware and must therefore be set up identically, which stays true if
// the args themselves are ever revised.
func TestHardwarePlumbing_HevcMatchesItsH264Sibling(t *testing.T) {
	for _, base := range H264Engines {
		hevc := HEVCVariant(base)
		if hevc == base {
			t.Fatalf("%s has no HEVC sibling — HEVCVariant and H264Engines have drifted apart", base)
		}

		if want, got := DeviceInitArgs(base), DeviceInitArgs(hevc); !slices.Equal(want, got) {
			t.Errorf("%s device init = %v, want %v (same engine as %s)", hevc, got, want, base)
		}
		if want, got := UploadFilter(base), UploadFilter(hevc); want != got {
			t.Errorf("%s upload filter = %q, want %q (same engine as %s)", hevc, got, want, base)
		}
		if want, got := DecodeArgs(base), DecodeArgs(hevc); !slices.Equal(want, got) {
			t.Errorf("%s decode args = %v, want %v (same engine as %s)", hevc, got, want, base)
		}
	}
}

// The three engines that need an explicit device must actually name one, in BOTH codecs.
//
// The pairwise test above would stay green if a refactor emptied both halves of a pair at once —
// which is close to the state this change found, since every HEVC encoder returned nothing. This
// pins the content, so "plumbing disappeared everywhere" cannot read as agreement.
func TestHardwarePlumbing_DeviceEnginesNameADevice(t *testing.T) {
	for _, base := range []Encoder{EncoderVAAPI, EncoderQSV, EncoderVulkan} {
		for _, enc := range []Encoder{base, HEVCVariant(base)} {
			if got := DeviceInitArgs(enc); len(got) == 0 {
				t.Errorf("%s got no device-init args; it cannot open its encode context without one", enc)
			}
		}
	}
	// NVENC takes no device init but MUST get hardware decode — losing `-hwaccel cuda` is the
	// measured 341%→0% CPU regression on 4K sources.
	for _, enc := range []Encoder{EncoderNVENC, EncoderNVENCHEVC} {
		if got := strings.Join(DecodeArgs(enc), " "); !strings.Contains(got, "cuda") {
			t.Errorf("%s lost its CUDA hardware decode: %q", enc, got)
		}
	}
}

// The decode rule every certified family shares: a codec outside the GPU's set, a chroma other
// than 4:2:0, 10-bit H.264 and an unknown codec all take the CPU decode, each with its reason.
func TestDecodeRefusal_NamesWhyTheGPUDoesNotDecode(t *testing.T) {
	nv := HostFor(EncoderNVENC, false, GPUFilters{})
	for _, c := range []struct {
		codec, pixfmt string
		tenBit        bool
		want          string
	}{
		{"h264", "yuv420p", false, ""},
		{"HEVC", "yuv420p10le", true, ""},
		{"", "yuv420p", false, "source codec unknown"},
		{"prores", "yuv422p10le", true, "the GPU does not decode prores"},
		{"h264", "yuv422p", false, "the GPU does not decode yuv422p"},
		{"h264", "yuv420p10le", true, "the GPU does not decode 10-bit H.264"},
	} {
		if got := nv.DecodeRefusal(c.codec, c.pixfmt, c.tenBit); got != c.want {
			t.Errorf("DecodeRefusal(%q, %q, %v) = %q, want %q", c.codec, c.pixfmt, c.tenBit, got, c.want)
		}
	}
}
