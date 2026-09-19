package api

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/mediatools"
)

func TestClipMediaDTOProjectsPreparedPlaybackFacts(t *testing.T) {
	tags := filler.SidecarTags{MediaAssets: &filler.MediaAssetManifest{
		Playback: &filler.MediaDerivativeLineage{
			Asset: filler.MediaAssetIdentity{Bytes: 9_361_105},
			Recipe: mediatools.DerivativeRecipe{
				Container: "mp4", VideoCodec: "h264", AudioCodec: "aac",
				AudioChannels: 2, AudioRateHz: 48_000,
			},
			OutputProbe: mediatools.Probed{Width: 1920, Height: 1080, Cadence: "24000/1001"},
		},
	}}

	want := &ClipMediaDTO{
		Width: 1920, Height: 1080, FrameRate: "24000/1001", Bytes: 9_361_105,
		Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", AudioChannels: 2, AudioRateHz: 48_000,
	}
	if got := clipMediaDTO(tags); !reflect.DeepEqual(got, want) {
		t.Fatalf("clipMediaDTO() = %+v, want %+v", got, want)
	}
}

func TestClipMediaDTOKeepsMissingPlaybackQuiet(t *testing.T) {
	if got := clipMediaDTO(filler.SidecarTags{}); got != nil {
		t.Fatalf("clipMediaDTO() = %+v, want nil", got)
	}
}
