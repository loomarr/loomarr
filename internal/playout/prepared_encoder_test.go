package playout

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/prepared"
)

func TestPreparedVideoArgsReuseNVENCPolicy(t *testing.T) {
	r := CanonicalPreparedRendition(TierBalanced)
	args, err := PreparedVideoArgs(EncoderNVENC, r)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args.OutputArgs, " ")
	for _, want := range []string{
		"-vf scale=1920:1080", "format=yuv420p", "-c:v h264_nvenc", "-preset p7",
		"-tune hq", "-profile:v high", "-level:v 4.1",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("prepared NVENC args missing %q: %s", want, joined)
		}
	}
	if got := strings.Join(args.InputArgs, " "); got != "-hwaccel cuda" {
		t.Fatalf("NVENC input args = %q, want GPU decode", got)
	}
}

func TestPreparedVideoArgsRejectSoftwareAndPlacesDeviceSetupBeforeInput(t *testing.T) {
	r := CanonicalPreparedRendition(TierBalanced)
	if _, err := PreparedVideoArgs(EncoderSoftware, r); err == nil {
		t.Fatal("PreparedVideoArgs accepted software for hardware-background work")
	}
	for _, encoder := range []Encoder{EncoderQSV, EncoderVAAPI, EncoderVulkan} {
		plan, err := PreparedVideoArgs(encoder, r)
		if err != nil {
			t.Errorf("PreparedVideoArgs(%q): %v", encoder, err)
			continue
		}
		if len(plan.InputArgs) == 0 {
			t.Errorf("PreparedVideoArgs(%q) omitted global device setup", encoder)
		}
	}
}

func TestPreparedVideoArgsRejectsAContractItCannotProduce(t *testing.T) {
	r := CanonicalPreparedRendition(TierBalanced)
	r.VideoCodec = "hevc"
	if _, err := PreparedVideoArgs(EncoderNVENC, r); !errors.Is(err, prepared.ErrUnsupportedRendition) {
		t.Fatalf("PreparedVideoArgs error = %v, want ErrUnsupportedRendition", err)
	}
}

// A prepared publication of an HDR source must carry exactly the chain live emits for the same
// source and build, in the same place. Building the live expectation from ProgramSpec keeps the
// test tied to the live decision rather than to a second copy of the string.
func TestPreparedVideoArgsToneMapMatchesLiveChain(t *testing.T) {
	r := CanonicalPreparedRendition(TierBalanced)
	r.ToneMap = true
	live := ProgramSpec{Source: MediaFormat{ColorTransfer: "smpte2084"}, Tonemap: true}
	if live.tonemapStep() == "" {
		t.Fatal("live playout does not tone-map a PQ source on a capable build; fixture is wrong")
	}
	for _, encoder := range []Encoder{EncoderNVENC, EncoderQSV, EncoderVAAPI, EncoderVulkan} {
		plan, err := PreparedVideoArgs(encoder, r)
		if err != nil {
			t.Fatalf("PreparedVideoArgs(%q): %v", encoder, err)
		}
		profile := Profile{
			Width: r.Width, Height: r.Height, Framerate: r.FrameRate, Encoder: encoder,
		}
		want := profile.scaleFilterArgs(live.tonemapStep())
		if !slices.Equal(plan.OutputArgs[:len(want)], want) {
			t.Errorf("%s: prepared filter = %v, want live's %v", encoder, plan.OutputArgs[:len(want)], want)
		}
		if strings.Count(strings.Join(plan.OutputArgs, " "), hdrToSDRChain) != 1 {
			t.Errorf("%s: tone-map chain not emitted exactly once: %v", encoder, plan.OutputArgs)
		}
	}
}

func TestPreparedVideoArgsSDRHasNoToneMapChain(t *testing.T) {
	r := CanonicalPreparedRendition(TierBalanced)
	plan, err := PreparedVideoArgs(EncoderNVENC, r)
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(plan.OutputArgs, " "); strings.Contains(joined, "zscale") || strings.Contains(joined, "tonemap") {
		t.Fatalf("SDR prepared args tone-map: %s", joined)
	}
}

// The decision is live's own: content property AND build capability, neither alone.
func TestToneMapAppliesNeedsHDRSourceAndCapableBuild(t *testing.T) {
	for _, tc := range []struct{ hdr, build, want bool }{
		{true, true, true}, {true, false, false}, {false, true, false}, {false, false, false},
	} {
		if got := ToneMapApplies(tc.hdr, tc.build); got != tc.want {
			t.Errorf("ToneMapApplies(hdr=%v, build=%v) = %v, want %v", tc.hdr, tc.build, got, tc.want)
		}
	}
}
