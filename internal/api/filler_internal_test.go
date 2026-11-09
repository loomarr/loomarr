package api

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The wire's needsDetails is the domain predicate, not a client-side guess from kind + tagged: an
// untagged bumper is complete, an untagged commercial is the exception.
func TestClipToDTONeedsDetailsFollowsThePredicate(t *testing.T) {
	cases := []struct {
		name string
		clip filler.Clip
		want bool
	}{
		{"tagged commercial", filler.Clip{Role: filler.Commercial, Era: 1991, Audience: filler.Kids, Category: "toys"}, false},
		{"commercial missing category", filler.Clip{Role: filler.Commercial, Era: 1991, Audience: filler.Kids}, true},
		{"bare commercial", filler.Clip{Role: filler.Commercial}, true},
		{"bare bumper", filler.Clip{Role: filler.Bumper}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clipToDTO(clipstore.Clip{Clip: tc.clip}, false, nil).NeedsDetails; got != tc.want {
				t.Errorf("needsDetails = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClipMediaDTOProjectsPreparedPlaybackFacts(t *testing.T) {
	tags := asset.SidecarTags{MediaAssets: &asset.MediaAssetManifest{
		Playback: &asset.MediaDerivativeLineage{
			Asset: asset.MediaAssetIdentity{Bytes: 9_361_105},
			Recipe: tools.DerivativeRecipe{
				Container: "mp4", VideoCodec: "h264", AudioCodec: "aac",
				AudioChannels: 2, AudioRateHz: 48_000,
			},
			OutputProbe: tools.Probed{Width: 1920, Height: 1080, Cadence: "24000/1001"},
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
	if got := clipMediaDTO(asset.SidecarTags{}); got != nil {
		t.Fatalf("clipMediaDTO() = %+v, want nil", got)
	}
}
