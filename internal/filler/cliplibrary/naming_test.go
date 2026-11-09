package cliplibrary_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/clips"
)

// #1452: a clip with no source title is named from grounded evidence, in order, and the name
// quotes that evidence rather than paraphrasing it. Nothing here needs the LLM or vision.
func TestGroundedName_Ladder(t *testing.T) {
	for _, tc := range []struct {
		name       string
		clip       clips.Clip
		want       string
		wantSource cliplibrary.NameSource
	}{
		{"brand wins", clips.Clip{Role: clips.Commercial, Brand: "Acme Tires", Era: 1994,
			Transcript: "come on down to the big sale"}, "Acme Tires — 1994", cliplibrary.NameFromBrand},
		{"transcript opening is quoted", clips.Clip{Role: clips.Commercial, Era: 1994,
			Transcript: "[Music] true tales of the canyon patrol. Troopers close in on a desperate gunman."},
			"“True tales of the canyon patrol” — 1994", cliplibrary.NameFromTranscript},
		{"long opening is cut at a word", clips.Clip{Role: clips.Commercial,
			Transcript: "tonight at eight on channel nineteen the whole family gathers for a very special hour"},
			"“Tonight at eight on channel nineteen…”", cliplibrary.NameFromTranscript},
		{"wordless clip falls through to on-screen text", clips.Clip{Role: clips.Bumper,
			Transcript: clips.TranscriptNone, VisibleText: "\n  SATURDAY  MORNING FUN \nstay tuned"},
			"“SATURDAY MORNING FUN”", cliplibrary.NameFromScreenText},
		{"a lone word is not a title", clips.Clip{Role: clips.Commercial, Category: "fast_food",
			Transcript: "♪ yeah ♪"}, "Fast food commercial", cliplibrary.NameFromCategory},
		{"era only", clips.Clip{Role: clips.StationID, Era: 1988}, "1988 station id", cliplibrary.NameFromEra},
		{"nothing grounded", clips.Clip{Role: clips.Commercial}, "Untitled commercial", cliplibrary.NameUntitled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, source := cliplibrary.GroundedName(tc.clip)
			if got != tc.want || source != tc.wantSource {
				t.Fatalf("GroundedName = %q (%s), want %q (%s)", got, source, tc.want, tc.wantSource)
			}
		})
	}
}

// #1988: provenance decides, not shape. A filename-sourced name is withheld even when it is spaced
// and capitalised like a title; every other recorded source shows, slug-shaped or not.
func TestViewerName_ProvenanceDecides(t *testing.T) {
	for _, name := range []string{"Widgetmax Ad 1991", "Fizzbolt Summer Sale", "commercial-widgetmax-1991"} {
		if got := cliplibrary.ViewerName(name, clips.NameFromFilename); got != "" {
			t.Errorf("ViewerName(%q, filename) = %q, want it withheld", name, got)
		}
	}
	for _, source := range []clips.NameSource{
		clips.NameFromSource, clips.NameFromSidecar, clips.NameFromGrounded, clips.NameFromCurated,
	} {
		for _, name := range []string{"Widgetmax Ad 1991", "zx-qt"} {
			if got := cliplibrary.ViewerName(name, source); got != name {
				t.Errorf("ViewerName(%q, %s) = %q, want it shown", name, source, got)
			}
		}
		if got := cliplibrary.ViewerName("  ", source); got != "" {
			t.Errorf("ViewerName(blank, %s) = %q, want empty", source, got)
		}
	}
}

// #1980: with no recorded provenance ('unknown', or "" from an older caller), a filename stem is
// recognised by its shape. Every file-shaped name is withheld; a spaced, capitalised or grounded
// name passes unchanged. This is the fallback #1988 keeps for rows nothing has classified.
func TestViewerName_UnknownProvenanceWithholdsFileShapedNames(t *testing.T) {
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
		for _, unknown := range []clips.NameSource{clips.NameFromUnknown, ""} {
			if got := cliplibrary.ViewerName(name, unknown); got != "" {
				t.Errorf("ViewerName(%q, %q) = %q, want it withheld", name, unknown, got)
			}
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
		if got := cliplibrary.ViewerName(name, clips.NameFromUnknown); got != name {
			t.Errorf("ViewerName(%q, unknown) = %q, want the curated name unchanged", name, got)
		}
	}
}
