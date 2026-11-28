package hls

import (
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/media"
)

// Answer 1 (#1512 phase 2): every client gets the H.264 1080p SDR baseline; a browser on a channel
// whose profile encoder is HEVC must not be handed HEVC. ChannelOutput alone follows the profile's
// encoder, so the packager keys by format.
func TestFormatOutputBaselineIsH264OnAnHEVCProfile(t *testing.T) {
	p := playout.Resolve(playout.TierFor("balanced"), encode.EncoderNVENCHEVC, 0)
	if !playout.ChannelOutput(p).HEVC {
		t.Fatal("fixture: an HEVC profile's ChannelOutput should be HEVC")
	}
	out, ok := playout.FormatOutput(playout.FormatBaseline, p)
	if !ok || out.HEVC || out.HDR {
		t.Fatalf("baseline output %+v ok=%v: want H.264 SDR", out, ok)
	}
	pl, err := playout.Build(encode.HostFor(encode.EncoderNVENCHEVC, false, encode.GPUFilters{}), media.MediaFormat{VideoCodec: "h264", Width: 1920,
		Height: 1080, FrameRate: 25, PixelFormat: "yuv420p", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000}, out)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.Index(pl.VideoEncode, "-c:v"); i < 0 || pl.VideoEncode[i+1] != string(encode.EncoderNVENC) {
		t.Fatalf("baseline encoder args %q: want h264_nvenc", pl.VideoEncode)
	}
	for _, class := range []playout.FormatClass{playout.Format4KSDR, playout.Format4KHDR} {
		prem, ok := playout.FormatOutput(class, p)
		if want, _ := playout.PremiumOutput(class, out); !ok || prem != want {
			t.Errorf("%s output %+v, want PremiumOutput over the baseline %+v", class, prem, want)
		}
	}
	if _, ok := playout.FormatOutput("8k-av1", p); ok {
		t.Error("an unknown class resolved")
	}
}
