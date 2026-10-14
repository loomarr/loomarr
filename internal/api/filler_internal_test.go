package api

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/store"
)

// The wire's needsDetails is the domain predicate, not a client-side guess from kind + tagged: an
// untagged bumper is complete, an untagged commercial is the exception.
func TestClipToDTONeedsDetailsFollowsThePredicate(t *testing.T) {
	cases := []struct {
		name string
		clip filler.Clip
		want bool
	}{
		{"tagged commercial", filler.Clip{Kind: filler.Commercial, Era: 1991, Audience: filler.Kids, Category: "toys"}, false},
		{"commercial missing category", filler.Clip{Kind: filler.Commercial, Era: 1991, Audience: filler.Kids}, true},
		{"bare commercial", filler.Clip{Kind: filler.Commercial}, true},
		{"bare bumper", filler.Clip{Kind: filler.Bumper}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clipToDTO(store.Clip{Clip: tc.clip}, false, nil).NeedsDetails; got != tc.want {
				t.Errorf("needsDetails = %v, want %v", got, tc.want)
			}
		})
	}
}

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
