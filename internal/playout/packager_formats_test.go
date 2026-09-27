package playout

import (
	"slices"
	"testing"
)

// Answer 1 (#1512 phase 2): every client and plan gets the H.264 1080p SDR baseline; a PlanBaseline
// browser on a channel whose profile encoder is HEVC must not be handed HEVC. ChannelOutput alone
// follows the profile's encoder (today's HEVC-plan sessions), so the packager keys by format.
func TestFormatOutputBaselineIsH264OnAnHEVCProfile(t *testing.T) {
	p := Resolve(TierFor("balanced"), EncoderNVENCHEVC, 0)
	if !ChannelOutput(p).HEVC {
		t.Fatal("fixture: an HEVC profile's ChannelOutput should be HEVC")
	}
	out, ok := FormatOutput(FormatBaseline, p)
	if !ok || out.HEVC || out.HDR {
		t.Fatalf("baseline output %+v ok=%v: want H.264 SDR", out, ok)
	}
	pl, err := Build(HostFor(EncoderNVENCHEVC, false, GPUFilters{}), MediaFormat{VideoCodec: "h264", Width: 1920,
		Height: 1080, FrameRate: 25, PixelFormat: "yuv420p", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000}, out)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.Index(pl.VideoEncode, "-c:v"); i < 0 || pl.VideoEncode[i+1] != string(EncoderNVENC) {
		t.Fatalf("baseline encoder args %q: want h264_nvenc", pl.VideoEncode)
	}
	for _, class := range []FormatClass{Format4KSDR, Format4KHDR} {
		prem, ok := FormatOutput(class, p)
		if want, _ := PremiumOutput(class, out); !ok || prem != want {
			t.Errorf("%s output %+v, want PremiumOutput over the baseline %+v", class, prem, want)
		}
	}
	if _, ok := FormatOutput("8k-av1", p); ok {
		t.Error("an unknown class resolved")
	}
}
