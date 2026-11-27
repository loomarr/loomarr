package scan

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// A film named as its folder, then its release's notes, bracketed or not, is named as its folder,
// year and all, until a catalog names it; one whose name says more than its folder is its own.
func TestAFilmNamedAsItsFolderWithItsReleasesNotesIsNamedAsTheFolder(t *testing.T) {
	for rel, want := range map[string]string{
		"Gorse Hollow 2 (1990)/Gorse Hollow 2 (1990) - X265 EAC3 5.1.mkv":                        "Gorse Hollow 2 (1990)",
		"Tarn Ridge (1972)/Tarn Ridge (1972) [imdbid-tt9000158] - [Bluray-2160p][HDR10]-GRP.mkv": "Tarn Ridge (1972)",
		"Tarn Ridge (1972)/Tarn Ridge Revisited (1972) - X264.mkv":                               "Tarn Ridge Revisited (1972) - X264",
	} {
		if got := fallbackName(rel, mp.KindMovie, 0, true); got != want {
			t.Errorf("fallbackName(%q) = %q, want %q", rel, got, want)
		}
	}
}

// A film's copies in its own folder are one title, though only one is grounded: the copy no
// catalog names takes the grounded copy's name and year. A film named otherwise there, or by
// another year, is its own.
func TestAFilmsCopiesInItsFolderAreOneTitle(t *testing.T) {
	const grounded = "Harrow Fen (2011) [imdbid-tt9440001] - [Remux-1080p][DTS-HD MA 5.1][AVC]-GRP.mkv"
	doc := `<movie><title>Harrow Fen</title><year>2011</year><uniqueid type="imdb">tt9440001</uniqueid></movie>`
	for _, c := range []struct {
		name        string
		copies, own []string
	}{
		{"a copy by its notes", []string{"Harrow Fen (2011) - X265 EAC3 5.1.mkv"}, nil},
		{"a copy by its title", []string{"Harrow Fen.mkv"}, nil},
		{"other films", nil, []string{"Harrow Fen 2 (2013).mkv", "Harrow Fen (1985).mkv"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			h := newHarness(t)
			dir := "Harrow Fen (2011)/"
			h.file(dir+grounded, 30)
			for _, rel := range slices.Concat(c.copies, c.own) {
				h.file(dir+rel, 10)
			}
			p := filepath.Join(h.root, filepath.FromSlash(dir+grounded))
			if err := os.WriteFile(strings.TrimSuffix(p, ".mkv")+".nfo", []byte(doc), 0o644); err != nil {
				t.Fatal(err)
			}
			h.run()
			rows, err := h.st.LibraryFiles(ctx, h.root)
			if err != nil {
				t.Fatal(err)
			}
			items := map[string]string{}
			for _, r := range rows {
				items[filepath.Base(r.Path)] = r.ItemID
			}
			film := items[grounded]
			for _, name := range c.copies {
				if items[name] != film {
					t.Errorf("%s is item %q; want the grounded copy's %q", name, items[name], film)
				}
			}
			for _, name := range c.own {
				if items[name] == "" || items[name] == film {
					t.Errorf("%s is item %q; want a title of its own, not %q", name, items[name], film)
				}
			}
			it, _ := ProjectLibraryItem(h.item(fileOrigin(h.root, dir+grounded)))
			if it.Name != "Harrow Fen" || it.Year != 2011 {
				t.Errorf("the film is %q (%d); want Harrow Fen (2011)", it.Name, it.Year)
			}
			for _, id := range it.IDs {
				if id.Namespace == filmCopiesNamespace {
					t.Errorf("the film lists the scanner's own copy id %v", id)
				}
			}
		})
	}
}

// A file in a film's folder whose own NFO names another year's film is that film, not a copy: the
// 1985 original kept in the 2011 remake's folder stays its own title, and joins its own folder's
// item when there is one, while a copy no NFO describes joins the remake.
func TestAFilmsOwnNFOOutranksItsFoldersCopies(t *testing.T) {
	ctx := context.Background()
	nfo := func(tmdb, year string) string {
		return `<movie><title>Harrow Fen</title><year>` + year + `</year><uniqueid type="tmdb" default="true">` + tmdb + `</uniqueid></movie>`
	}
	h := newHarness(t)
	files := map[string]string{
		"Harrow Fen (2011)/Harrow Fen (2011) - 1080p.mkv": nfo("90045222", "2011"),
		"Harrow Fen (2011)/Harrow Fen (2011) - x265.mkv":  "",
		"Harrow Fen (2011)/Harrow Fen.mkv":                nfo("90045111", "1985"),
		"Harrow Fen (1985)/Harrow Fen (1985).mkv":         nfo("90045111", "1985"),
	}
	for rel, doc := range files {
		p := h.file(rel, 10)
		if doc != "" {
			if err := os.WriteFile(strings.TrimSuffix(p, ".mkv")+".nfo", []byte(doc), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	h.run()
	rows, err := h.st.LibraryFiles(ctx, h.root)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]string{}
	for _, r := range rows {
		rel, _ := filepath.Rel(h.root, r.Path)
		item[filepath.ToSlash(rel)] = r.ItemID
		if r.Problem != "" {
			t.Errorf("%s has problem %q", rel, r.Problem)
		}
	}
	remake, original := item["Harrow Fen (2011)/Harrow Fen (2011) - 1080p.mkv"], item["Harrow Fen (1985)/Harrow Fen (1985).mkv"]
	if remake == "" || original == "" || remake == original {
		t.Fatalf("remake %q, original %q; want two titles", remake, original)
	}
	if got := item["Harrow Fen (2011)/Harrow Fen (2011) - x265.mkv"]; got != remake {
		t.Errorf("the undescribed copy is item %q; want the remake's %q", got, remake)
	}
	if got := item["Harrow Fen (2011)/Harrow Fen.mkv"]; got != original {
		t.Errorf("the 1985 file in the remake's folder is item %q; want the original's %q", got, original)
	}
}

// Two files in one film's folder that their own NFOs ground as two films by their catalog ids are
// two titles, however alike their titles and years (a sequel a year on, or a second film of the
// same name and year), and whether the second is found in the first's pass or a later one. Two
// copies whose NFOs name the film by different catalogs are one title.
func TestFilesTheCatalogGroundsAsTwoFilmsAreTwoTitles(t *testing.T) {
	nfo := func(id, title, year string) string {
		kind := "tmdb"
		if strings.HasPrefix(id, "tt") {
			kind = "imdb"
		}
		return `<movie><title>` + title + `</title><year>` + year + `</year><uniqueid type="` + kind + `" default="true">` + id + `</uniqueid></movie>`
	}
	for _, c := range []struct {
		name, second string
		later        bool
		titles       int
	}{
		{"a sequel a year on", nfo("90046333", "Harrow Fen II", "2012"), false, 2},
		{"same title, another film", nfo("90046444", "Harrow Fen", "2011"), false, 2},
		{"a sequel found a pass later", nfo("90046333", "Harrow Fen II", "2012"), true, 2},
		{"another film found a pass later", nfo("90046444", "Harrow Fen", "2011"), true, 2},
		{"the film by another catalog", nfo("tt9046222", "Harrow Fen", "2011"), false, 1},
		{"the film by another catalog a pass later", nfo("tt9046222", "Harrow Fen", "2011"), true, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			h := newHarness(t)
			add := func(rel, doc string) {
				p := h.file(rel, 10)
				if err := os.WriteFile(strings.TrimSuffix(p, ".mkv")+".nfo", []byte(doc), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			add("Harrow Fen (2011)/Harrow Fen (2011).mkv", nfo("90046222", "Harrow Fen", "2011"))
			if c.later {
				h.run()
				h.run()
			}
			add("Harrow Fen (2011)/Harrow Fen.mkv", c.second)
			h.run()
			h.run()
			rows, err := h.st.LibraryFiles(ctx, h.root)
			if err != nil {
				t.Fatal(err)
			}
			items := map[string]bool{}
			for _, r := range rows {
				if r.ItemID == "" || r.Problem != "" {
					t.Errorf("%s: item %q, problem %q", r.Path, r.ItemID, r.Problem)
				}
				items[r.ItemID] = true
			}
			if len(items) != c.titles {
				t.Errorf("%d titles; want %d", len(items), c.titles)
			}
		})
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
	detail, err := lib.Item(ctx, string(item.ID), false)
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
