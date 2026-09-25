package playout

import (
	"testing"

	"github.com/loomarr/loomarr/internal/prepared"
)

// The prepared storage reservation is sized from the ceiling of the encoder actually launched, so
// this pins that the NVENC plan's -maxrate is what the packager reports (it is read from the real
// flags, not a second copy of the ladder).
func TestPreparedNVENCRateCeilingIsTwiceTheRungAndDrivesTheReservation(t *testing.T) {
	rendition := CanonicalPreparedRendition(TierFor("balanced"))
	packager := prepared.NewFFmpegPackager("ffmpeg", func(r prepared.RenditionContract) (prepared.VideoPlan, error) {
		return PreparedVideoArgs(EncoderNVENC, r)
	})
	ceiling := packager.VideoRateCeilingKbps(rendition)
	if want := rendition.VideoBitrateKbps * 2; ceiling != want {
		t.Fatalf("NVENC prepared ceiling = %d kbit/s, want 2x the %d rung = %d",
			ceiling, rendition.VideoBitrateKbps, want)
	}
}

func TestPreparedSoftwareFamilyPlansStateNoCeilingBeyondTheirRung(t *testing.T) {
	rendition := CanonicalPreparedRendition(TierFor("balanced"))
	packager := prepared.NewFFmpegPackager("ffmpeg", func(r prepared.RenditionContract) (prepared.VideoPlan, error) {
		return PreparedVideoArgs(EncoderVAAPI, r)
	})
	if got := packager.VideoRateCeilingKbps(rendition); got > rendition.VideoBitrateKbps {
		t.Fatalf("bitrate-targeted VAAPI ceiling = %d, want no more than the %d rung", got, rendition.VideoBitrateKbps)
	}
}
