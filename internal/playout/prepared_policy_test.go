package playout

import (
	"testing"

	"github.com/loomarr/loomarr/internal/prepared"
)

func TestCanonicalPreparedRenditionUsesTheTierTopRung(t *testing.T) {
	for _, tier := range []Tier{TierEfficient, TierBalanced, TierQuality} {
		t.Run(string(tier), func(t *testing.T) {
			got := CanonicalPreparedRendition(tier)
			wantProfile := Resolve(tier, EncoderSoftware, 2, 0)
			if got.Width != wantProfile.Width || got.Height != wantProfile.Height ||
				got.FrameRate != wantProfile.Framerate ||
				got.VideoBitrateKbps != wantProfile.VideoBitrate ||
				got.AudioBitrateKbps != wantProfile.AudioBitrate {
				t.Fatalf("CanonicalPreparedRendition(%q) = %+v, want top rung %+v", tier, got, wantProfile)
			}
		})
	}
}

func TestCanonicalPreparedRenditionIsPortableAndVersioned(t *testing.T) {
	got := CanonicalPreparedRendition(TierBalanced)
	if got.VideoCodec != "h264" || got.VideoProfile != "high" || got.VideoLevel != "4.1" ||
		got.PixelFormat != "yuv420p" || got.HDR != "sdr" || got.AudioCodec != "aac" ||
		got.AudioLayout != "stereo" {
		t.Fatalf("canonical rendition is not the portable h264/aac contract: %+v", got)
	}
	if got.SegmentDurationMS != 2_000 {
		t.Fatalf("segment duration = %dms, want 2000ms", got.SegmentDurationMS)
	}
	if got.PackagingVersion != prepared.CurrentPackagingVersion {
		t.Fatalf("packaging version = %d, want %d", got.PackagingVersion, prepared.CurrentPackagingVersion)
	}
}
