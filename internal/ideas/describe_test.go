package ideas_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/ideas"
)

func TestDescribe_NamesAndPitchesFromFacetsAlone(t *testing.T) {
	cases := []struct {
		name      string
		idea      ideas.Idea
		label     string
		wantName  string
		wantPitch string
	}{
		{
			name:      "genre, movies only, capitalised",
			idea:      ideas.Idea{Facet: ideas.FacetGenre, Value: "comedy", Movies: 12},
			wantName:  "Comedy Movies",
			wantPitch: "Comedy from your library that no channel plays yet: 12 movies.",
		},
		{
			name:      "genre keeps the library's spelling",
			idea:      ideas.Idea{Facet: ideas.FacetGenre, Value: "Sci-Fi & Fantasy", Series: 7},
			wantName:  "Sci-Fi & Fantasy Shows",
			wantPitch: "Sci-Fi & Fantasy from your library that no channel plays yet: 7 shows.",
		},
		{
			name:      "decade last century, mixed",
			idea:      ideas.Idea{Facet: ideas.FacetDecade, Value: "1990", Movies: 9, Series: 1},
			wantName:  "90s Channel",
			wantPitch: "Titles from the 1990s in your library that no channel plays yet: 9 movies and 1 show.",
		},
		{
			name:      "decade this century",
			idea:      ideas.Idea{Facet: ideas.FacetDecade, Value: "2010", Movies: 1},
			wantName:  "2010s Movies",
			wantPitch: "Titles from the 2010s in your library that no channel plays yet: 1 movie.",
		},
		{
			name:      "holiday uses the calendar's label",
			idea:      ideas.Idea{Facet: ideas.FacetHoliday, Value: "halloween", Movies: 4, Series: 2},
			label:     "Halloween",
			wantName:  "Halloween Channel",
			wantPitch: "Titles about Halloween from your library, for the season: 4 movies and 2 shows.",
		},
		{
			name:      "holiday without a label falls back to its id",
			idea:      ideas.Idea{Facet: ideas.FacetHoliday, Value: "halloween", Movies: 3},
			wantName:  "Halloween Movies",
			wantPitch: "Titles about Halloween from your library, for the season: 3 movies.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, pitch := ideas.Describe(tc.idea, tc.label)
			if name != tc.wantName || pitch != tc.wantPitch {
				t.Fatalf("Describe = (%q, %q), want (%q, %q)", name, pitch, tc.wantName, tc.wantPitch)
			}
		})
	}
}
