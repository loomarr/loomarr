package filler_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

// #1452: a clip with no source title is named from grounded evidence, in order, and the name
// quotes that evidence rather than paraphrasing it. Nothing here needs the LLM or vision.
func TestGroundedName_Ladder(t *testing.T) {
	for _, tc := range []struct {
		name       string
		clip       filler.Clip
		want       string
		wantSource filler.NameSource
	}{
		{"brand wins", filler.Clip{Kind: filler.Commercial, Brand: "Acme Tires", Era: 1994,
			Transcript: "come on down to the big sale"}, "Acme Tires — 1994", filler.NameFromBrand},
		{"transcript opening is quoted", filler.Clip{Kind: filler.Commercial, Era: 1994,
			Transcript: "[Music] real stories of the highway patrol. Troopers close in on a desperate gunman."},
			"“Real stories of the highway patrol” — 1994", filler.NameFromTranscript},
		{"long opening is cut at a word", filler.Clip{Kind: filler.Commercial,
			Transcript: "tonight at eight on channel four the whole family gathers for a very special hour"},
			"“Tonight at eight on channel four…”", filler.NameFromTranscript},
		{"wordless clip falls through to on-screen text", filler.Clip{Kind: filler.Bumper,
			Transcript: filler.TranscriptNone, VisibleText: "\n  SATURDAY  MORNING FUN \nstay tuned"},
			"“SATURDAY MORNING FUN”", filler.NameFromScreenText},
		{"a lone word is not a title", filler.Clip{Kind: filler.Commercial, Category: "fast_food",
			Transcript: "♪ yeah ♪"}, "Fast food commercial", filler.NameFromCategory},
		{"era only", filler.Clip{Kind: filler.StationID, Era: 1988}, "1988 station id", filler.NameFromEra},
		{"nothing grounded", filler.Clip{Kind: filler.Commercial}, "Untitled commercial", filler.NameUntitled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, source := filler.GroundedName(tc.clip)
			if got != tc.want || source != tc.wantSource {
				t.Fatalf("GroundedName = %q (%s), want %q (%s)", got, source, tc.want, tc.wantSource)
			}
		})
	}
}
