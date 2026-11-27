package pod_test

import (
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

// Audience is strict for kids only (#3838 decision 7): a kids channel airs a kids or family clip
// only when more than one model's guess stands behind that audience. Every other channel takes a
// single model's guess as the audience.
func TestKidsChannelAirsOnlyCorroboratedKidsAndFamilyClips(t *testing.T) {
	ad := func(id string, aud pod.Audience, guess bool) pod.Clip {
		return pod.Clip{Hash: id, Path: id, Role: pod.Commercial, Era: 1992, Audience: aud,
			AudienceGuess: guess, DurationMs: 20_000}
	}
	bookend := pod.Clip{Hash: "guess-ident", Path: "guess-ident", Role: pod.StationID, Era: 1992,
		Audience: pod.Kids, AudienceGuess: true, DurationMs: 5_000}
	catalog := []pod.Clip{
		ad("kids-cued", pod.Kids, false), ad("family-cued", pod.Family, false),
		ad("kids-guess", pod.Kids, true), ad("family-guess", pod.Family, true),
		ad("general-guess", pod.General, true), bookend,
	}
	for _, tc := range []struct {
		channel pod.Audience
		want    []string
	}{
		{pod.Kids, []string{"family-cued", "kids-cued"}},
		{pod.Family, []string{"family-cued", "family-guess", "general-guess", "guess-ident", "kids-cued", "kids-guess"}},
		{pod.General, []string{"family-cued", "family-guess", "general-guess", "guess-ident", "kids-cued", "kids-guess"}},
	} {
		w := pod.Window{ChannelID: "c", Era: pod.Year(1992), Audience: tc.channel, GapMs: 400_000, PodMax: 8}
		if got := airedHashes(catalog, w); !slices.Equal(got, tc.want) {
			t.Errorf("%s channel aired %v, want %v", tc.channel, got, tc.want)
		}
	}
}
