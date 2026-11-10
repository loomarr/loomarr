package arr

import (
	"reflect"
	"testing"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// The arrs name art in their own words; library_art's CHECK holds only Loomarr's, so every kind
// that reaches a record is mapped and the rest are left out.
func TestArtworkMapsTheArrsCoverTypesOntoLoomarrsKinds(t *testing.T) {
	got := artwork([]image{
		{CoverType: "poster", RemoteURL: "https://a/poster.jpg"},
		{CoverType: "Clearlogo", RemoteURL: "https://a/logo.png"},
		{CoverType: "screenshot", RemoteURL: "https://a/still.jpg"},
		{CoverType: "headshot", RemoteURL: "https://a/face.jpg"},
		{CoverType: "unknown", RemoteURL: "https://a/what.jpg"},
		{CoverType: "banner", RemoteURL: "/MediaCover/1/banner.jpg"},
	})
	want := []mp.ArtworkRef{
		{Kind: mp.KindPoster, Ref: "https://a/poster.jpg"},
		{Kind: mp.KindClearlogo, Ref: "https://a/logo.png"},
		{Kind: mp.KindThumb, Ref: "https://a/still.jpg"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("artwork = %+v, want %+v", got, want)
	}
}
