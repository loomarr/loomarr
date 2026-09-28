package playout

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// Arg-shape tests are cheap and run everywhere. The ones that actually EXECUTE ffmpeg
// live in *_live_test.go files behind a build tag, because unit tests must not depend on
// a binary being present (AGENTS.md: unit tests never touch the network, and the same
// spirit applies to external executables).

func argsAfter(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// FindFont must not invent a path: a non-existent fontfile is the init failure above.
func TestFindFont_ReturnsOnlyAnExistingFile(t *testing.T) {
	got := FindFont()
	if got == "" {
		t.Skip("no font on this system — the degrade path is covered above")
	}
	if _, err := exec.LookPath("test"); err == nil { // trivially available
		if !fileExists(got) {
			t.Errorf("FindFont returned %q which does not exist", got)
		}
	}
}

// AN HEVC ENCODER MUST GET ITS ENGINE'S HARDWARE PLUMBING (§9.1 V49).
//
// deviceInitArgs, hardwareUploadFilter and hardwareDecodeArgs each switch on the encoder. All
// three listed only the h264 constants, so every hevc_* encoder fell through to `default` and
// received nothing: no `-vaapi_device`, no `-init_hw_device` for QSV/Vulkan, no `-hwaccel cuda`
// for NVENC.
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
	for _, base := range h264Engines {
		hevc := hevcVariant(base)
		if hevc == base {
			t.Fatalf("%s has no HEVC sibling — hevcVariant and h264Engines have drifted apart", base)
		}

		if want, got := deviceInitArgs(base), deviceInitArgs(hevc); !slices.Equal(want, got) {
			t.Errorf("%s device init = %v, want %v (same engine as %s)", hevc, got, want, base)
		}
		if want, got := hardwareUploadFilter(base), hardwareUploadFilter(hevc); want != got {
			t.Errorf("%s upload filter = %q, want %q (same engine as %s)", hevc, got, want, base)
		}
		if want, got := hardwareDecodeArgs(base), hardwareDecodeArgs(hevc); !slices.Equal(want, got) {
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
		for _, enc := range []Encoder{base, hevcVariant(base)} {
			if got := deviceInitArgs(enc); len(got) == 0 {
				t.Errorf("%s got no device-init args; it cannot open its encode context without one", enc)
			}
		}
	}
	// NVENC takes no device init but MUST get hardware decode — losing `-hwaccel cuda` is the
	// measured 341%→0% CPU regression on 4K sources.
	for _, enc := range []Encoder{EncoderNVENC, EncoderNVENCHEVC} {
		if got := strings.Join(hardwareDecodeArgs(enc), " "); !strings.Contains(got, "cuda") {
			t.Errorf("%s lost its CUDA hardware decode: %q", enc, got)
		}
	}
}
