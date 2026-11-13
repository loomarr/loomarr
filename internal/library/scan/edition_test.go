package scan

import (
	"context"
	"testing"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// An edition is shown as its source gave it, fixed only when it came in one case; the theatrical
// version is the default and has no label. Labels that differ only by "Cut", case or an apostrophe
// are one edition.
func TestEditionLabel(t *testing.T) {
	for in, want := range map[string]string{
		"":                   "",
		"  Directors   Cut ": "Directors Cut",
		"EXTENDED":           "Extended",
		"director's cut":     "Director's Cut",
		"IMAX Enhanced":      "IMAX Enhanced",
		"THEATRICAL":         "",
		"Theatrical Cut":     "",
	} {
		if got := editionLabel(in); got != want {
			t.Errorf("editionLabel(%q) = %q, want %q", in, got, want)
		}
	}
	for _, same := range [][2]string{{"Extended", "EXTENDED Cut"}, {"Director's Cut", "Directors Cut"}, {"Uncut", "UNCUT"}} {
		if editionKey(same[0]) != editionKey(same[1]) {
			t.Errorf("%q and %q are different editions, want one", same[0], same[1])
		}
	}
	if editionKey("Extended") == editionKey("Unrated") {
		t.Error("Extended and Unrated are one edition")
	}
}

// A duplicated file is named as its original, so a film's name never depends on which of its
// copies names the item (#4010); a title that holds the word keeps it.
func TestADuplicatedFileIsNamedAsItsOriginal(t *testing.T) {
	for rel, want := range map[string]string{
		"Kilo (2016) {tmdb-90000301}/Kilo (2016) {tmdb-90000301} copy.mkv": "Kilo (2016)",
		"Kilo (2016)/Kilo (2016) copy 2.mkv":                               "Kilo (2016)",
		"Kilo (2016)/Kilo (2016) - Copy (2).mkv":                           "Kilo (2016)",
		"Kilo (2016)/Kilo (2016) - Copy.mkv":                               "Kilo (2016)",
		"Certified Copy (2010)/Certified Copy (2010).mkv":                  "Certified Copy (2010)",
	} {
		if got := fallbackName(rel, mp.KindMovie, 0, false); got != want {
			t.Errorf("fallbackName(%q) = %q, want %q", rel, got, want)
		}
	}
}

// Two editions of one movie are one title, each with a copy that plays; a second copy of the
// default edition is still an extra. The edition tag never reaches the title's name.
func TestEachEditionHasItsOwnPlayingCopy(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	dir := "Kilo (2016) {tmdb-90000301}/"
	h.file(dir+"Kilo (2016) {tmdb-90000301}.mkv", 30)
	h.file(dir+"Kilo (2016) {tmdb-90000301} copy.mkv", 10)
	cut := h.file(dir+"Kilo (2016) {tmdb-90000301} {edition-DIRECTORS CUT}.mkv", 20)
	h.run()

	item := h.item(fileOrigin(h.root, dir+"Kilo (2016) {tmdb-90000301}.mkv"))
	sources := librarySources(item)
	if len(sources) != 3 {
		t.Fatalf("sources = %+v, want three copies of one title", sources)
	}
	plays := map[string]bool{}
	for _, s := range sources {
		plays[s.Edition] = plays[s.Edition] || s.Plays
		if s.Path == cut && s.Edition != "Directors Cut" {
			t.Errorf("edition of %s = %q, want Directors Cut", s.Path, s.Edition)
		}
	}
	if !plays[""] || !plays["Directors Cut"] {
		t.Errorf("plays by edition = %v, want one copy of each", plays)
	}

	lib := &Library{ItemStore: h.st, Inventory: h.inv}
	detail, err := lib.Item(ctx, string(item.ID))
	if err != nil {
		t.Fatal(err)
	}
	extras := 0
	for _, f := range detail.Files {
		if f.Extra {
			extras++
			if f.Edition != "" || f.Size != 10 {
				t.Errorf("extra = %s (%q, %d bytes), want the smaller default copy", f.Path, f.Edition, f.Size)
			}
		}
	}
	if extras != 1 {
		t.Errorf("extras = %d, want only the second default copy", extras)
	}
	if name := detail.Facts.Name; name != "Kilo (2016)" {
		t.Errorf("name = %q, want no edition tag in it", name)
	}
}
