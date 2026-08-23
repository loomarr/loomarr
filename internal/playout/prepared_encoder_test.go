package playout

import (
	"errors"
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
