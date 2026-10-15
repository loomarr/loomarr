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
			Transcript: "[Music] true tales of the canyon patrol. Troopers close in on a desperate gunman."},
			"“True tales of the canyon patrol” — 1994", filler.NameFromTranscript},
		{"long opening is cut at a word", filler.Clip{Kind: filler.Commercial,
			Transcript: "tonight at eight on channel nineteen the whole family gathers for a very special hour"},
			"“Tonight at eight on channel nineteen…”", filler.NameFromTranscript},
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

// #1980: a filename stem is an operator's handle, not viewer copy. Every file-shaped name is
// withheld; a spaced, capitalised or grounded name passes unchanged.
func TestViewerName_WithholdsFileShapedNames(t *testing.T) {
	for _, name := range []string{
		"commercial-widgetmax-1991",
		"commercial_widgetmax_1991",
		"Widgetmax_Ad_1991",
		"widgetmax-1991.mp4",
		"Widgetmax Ad 1991.MKV",
		"ads/widgetmax-1991",
		`ads\widgetmax`,
		"3f7a9c0e1b2d4f6a8c0e2b4d6f8a0c2e4b6d8f0a2c4e6b8d0f2a4c6e8b0d2f4a",
		"   ",
		"",
	} {
		if got := filler.ViewerName(name); got != "" {
			t.Errorf("ViewerName(%q) = %q, want it withheld", name, got)
		}
	}
	for _, name := range []string{
		"Fizzbolt",
		"Zip-Zap",
		"ZX/QT",
		"24/7 Fitness",
		"Bolt/Volt",
		"Widgetmax commercial 1991",
		"Acme Tires — 1994",
		"“True tales of the canyon patrol”",
		"Dr. Fizz",
		"Loomarr — We'll be right back",
	} {
		if got := filler.ViewerName(name); got != name {
			t.Errorf("ViewerName(%q) = %q, want the curated name unchanged", name, got)
		}
	}
}
