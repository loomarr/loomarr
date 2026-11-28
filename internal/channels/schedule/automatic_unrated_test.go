package schedule

import (
	"testing"

	"github.com/loomarr/loomarr/internal/platform/rating"
)

// The field answers "what would automatic do under this ceiling": kids ceilings (up to TV-PG/PG)
// skip unrated titles, everything else allows them, and an explicit choice changes nothing.
func TestAutomaticUnratedFollowsTheCeilingAlone(t *testing.T) {
	for ceiling, want := range map[rating.Rating]UnratedPolicy{
		"": UnratedAllow, "TV-Y": UnratedExclude, "TV-G": UnratedExclude, "PG": UnratedExclude,
		"TV-PG": UnratedExclude, "TV-14": UnratedAllow, "R": UnratedAllow,
	} {
		for _, explicit := range []UnratedPolicy{UnratedDefault, UnratedAllow, UnratedExclude} {
			p := ChannelPolicy{ProposalPolicy: ProposalPolicy{Audience: AudiencePolicy{Ceiling: ceiling, Unrated: explicit}}}
			got := p.WithAutomaticUnrated()
			if got.AutomaticUnrated != want || got.Audience.Unrated != explicit {
				t.Errorf("ceiling %q, explicit %q: automatic %q (want %q), stored choice %q", ceiling, explicit, got.AutomaticUnrated, want, got.Audience.Unrated)
			}
		}
	}
}

func TestMergeFromOperatorNeverStoresAutomaticUnrated(t *testing.T) {
	echoed := ChannelPolicy{AutomaticUnrated: UnratedExclude}
	if got := (ChannelPolicy{}).MergeFromOperator(echoed).AutomaticUnrated; got != "" {
		t.Fatalf("stored automaticUnrated = %q", got)
	}
}
