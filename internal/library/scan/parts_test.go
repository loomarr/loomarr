package scan

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A folder's videos are a movie in parts only as a whole set: two or more names the same but for
// a part mark, numbered from 1 with none missing. A part mark alone, or in a title's own name,
// makes no set.
func TestPartsOf(t *testing.T) {
	for _, c := range []struct {
		name  string
		names []string
		want  map[string]int
	}{
		{"cd", []string{"Ashgrove (2015) - cd1.mkv", "Ashgrove (2015) - cd2.mkv"},
			map[string]int{"Ashgrove (2015) - cd1.mkv": 1, "Ashgrove (2015) - cd2.mkv": 2}},
		{"three, out of order", []string{"Ashgrove.part3.avi", "Ashgrove.part1.avi", "Ashgrove.part2.avi"},
			map[string]int{"Ashgrove.part1.avi": 1, "Ashgrove.part2.avi": 2, "Ashgrove.part3.avi": 3}},
		{"letters", []string{"Ashgrove (2015) - pt A.mkv", "Ashgrove (2015) - pt B.mkv"},
			map[string]int{"Ashgrove (2015) - pt A.mkv": 1, "Ashgrove (2015) - pt B.mkv": 2}},
		{"bracketed", []string{"Ashgrove (2015) [disc 1].mkv", "Ashgrove (2015) [disc 2].mkv"},
			map[string]int{"Ashgrove (2015) [disc 1].mkv": 1, "Ashgrove (2015) [disc 2].mkv": 2}},
		{"glued to the year", []string{"Ashgrove (2015)cd1.mkv", "Ashgrove (2015)cd2.mkv"},
			map[string]int{"Ashgrove (2015)cd1.mkv": 1, "Ashgrove (2015)cd2.mkv": 2}},
		{"words after the mark", []string{"Ashgrove (2015) - dvd1 - 1080p.mkv", "Ashgrove (2015) - dvd2 - 1080p.mkv"},
			map[string]int{"Ashgrove (2015) - dvd1 - 1080p.mkv": 1, "Ashgrove (2015) - dvd2 - 1080p.mkv": 2}},
		{"beside another movie", []string{"Ashgrove - cd1.mkv", "Ashgrove - cd2.mkv", "Brindle.mkv"},
			map[string]int{"Ashgrove - cd1.mkv": 1, "Ashgrove - cd2.mkv": 2}},
		{"a title's own part, alone", []string{"Kettle Part 2 (2011).mkv"}, nil},
		{"two titles' own parts", []string{"Kettle Part 1 (2010).mkv", "Kettle Part 2 (2011).mkv"}, nil},
		// "Part" straight after a title word is the title's: sequels, not a movie in parts. A title
		// never writes "pt", and a year or a release's mark before "Part" ends the title.
		{"two titles' own parts, no year", []string{"Harbor Saga Part 1.mkv", "Harbor Saga Part 2.mkv"}, nil},
		// A release's mark straight after the part's number is a rip's: a sequel gives its year first.
		{"part then a release's mark", []string{"Ashgrove Part 1 1080p.mkv", "Ashgrove Part 2 1080p.mkv"},
			map[string]int{"Ashgrove Part 1 1080p.mkv": 1, "Ashgrove Part 2 1080p.mkv": 2}},
		{"sequels, their years before the mark", []string{"Harbor Saga Part 1 (2010) 1080p.mkv", "Harbor Saga Part 2 (2011) 1080p.mkv"}, nil},
		{"pts after a title word", []string{"Harbor Saga pt 1.mkv", "Harbor Saga pt 2.mkv"},
			map[string]int{"Harbor Saga pt 1.mkv": 1, "Harbor Saga pt 2.mkv": 2}},
		{"pt joined", []string{"Ashgrove pt1.mkv", "Ashgrove pt2.mkv"},
			map[string]int{"Ashgrove pt1.mkv": 1, "Ashgrove pt2.mkv": 2}},
		{"part after a bare year", []string{"Ashgrove 2015 Part 1.mkv", "Ashgrove 2015 Part 2.mkv"},
			map[string]int{"Ashgrove 2015 Part 1.mkv": 1, "Ashgrove 2015 Part 2.mkv": 2}},
		{"part after a release mark", []string{"Ashgrove 1080p Part 1.mkv", "Ashgrove 1080p Part 2.mkv"},
			map[string]int{"Ashgrove 1080p Part 1.mkv": 1, "Ashgrove 1080p Part 2.mkv": 2}},
		{"part after a separator", []string{"Harbor Saga - Part 1.mkv", "Harbor Saga - Part 2.mkv"},
			map[string]int{"Harbor Saga - Part 1.mkv": 1, "Harbor Saga - Part 2.mkv": 2}},
		{"part after the year", []string{"Ashgrove (2015) Part 1.mkv", "Ashgrove (2015) Part 2.mkv"},
			map[string]int{"Ashgrove (2015) Part 1.mkv": 1, "Ashgrove (2015) Part 2.mkv": 2}},
		{"disc after a title word", []string{"Ashgrove Disc 1.mkv", "Ashgrove Disc 2.mkv"},
			map[string]int{"Ashgrove Disc 1.mkv": 1, "Ashgrove Disc 2.mkv": 2}},
		{"a part missing", []string{"Ashgrove - cd1.mkv", "Ashgrove - cd3.mkv"}, nil},
		{"a part twice", []string{"Ashgrove - cd1.mkv", "Ashgrove - cd1.avi", "Ashgrove - cd2.mkv"},
			map[string]int{"Ashgrove - cd1.mkv": 1, "Ashgrove - cd2.mkv": 2}},
		{"one part", []string{"Ashgrove - cd1.mkv"}, nil},
		{"numbers and letters", []string{"Ashgrove - pt1.mkv", "Ashgrove - ptB.mkv"}, nil},
		{"a mark inside a word", []string{"Counterpart1.mkv", "Counterpart2.mkv"}, nil},
		{"a word after the mark", []string{"Ashgrove - part about.mkv", "Ashgrove - part bout.mkv"}, nil},
		{"other names", []string{"Ashgrove - cd1.mkv", "Brindle - cd2.mkv"}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := partsOf("", c.names); !maps.Equal(got, c.want) {
				t.Errorf("partsOf(%q) = %v, want %v", c.names, got, c.want)
			}
		})
	}
}

// A part of a movie in parts with nothing else to name it is named as the movie, without its
// part mark; a whole file keeps a mark in its own name.
func TestAPartIsNamedWithoutItsMark(t *testing.T) {
	for _, c := range []struct {
		rel  string
		part int
		want string
	}{
		{"Ashgrove (2015) {tmdb-90000311}/Ashgrove (2015) {tmdb-90000311} - cd1.mkv", 1, "Ashgrove (2015)"},
		{"Ashgrove/Ashgrove.part2.avi", 2, "Ashgrove"},
		{"Ashgrove/Ashgrove (2015) [disc 1] 1080p.mkv", 1, "Ashgrove (2015) 1080p"},
		{"Kettle Part 2 (2011)/Kettle Part 2 (2011).mkv", 0, "Kettle Part 2 (2011)"},
	} {
		if got := fallbackName(c.rel, mp.KindMovie, c.part, true); got != c.want {
			t.Errorf("fallbackName(%q, part %d) = %q, want %q", c.rel, c.part, got, c.want)
		}
	}
}

// A movie in parts is one title whose every part plays, in part order, and whose length is its
// parts' together; the folder's poster is the movie's. Beside a whole copy of the same edition,
// the whole copy plays and the parts are extra copies.
func TestAMovieInPartsPlaysEveryPart(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	dir := "Ashgrove (2015) {tmdb-90000311}/"
	one := h.file(dir+"Ashgrove (2015) {tmdb-90000311} - cd1.mkv", 20)
	two := h.file(dir+"Ashgrove (2015) {tmdb-90000311} - cd2.mkv", 30)
	h.file(dir+"poster.jpg", 5)
	h.run()

	item := h.item(fileOrigin(h.root, dir+"Ashgrove (2015) {tmdb-90000311} - cd1.mkv"))
	sources := librarySources(item)
	if len(sources) != 2 {
		t.Fatalf("sources = %+v, want two parts of one title", sources)
	}
	for i, want := range []struct {
		path string
		part int
	}{{one, 1}, {two, 2}} {
		if s := sources[i]; s.Path != want.path || s.Part != want.part || !s.Plays {
			t.Errorf("source %d = %s part %d plays=%v, want %s part %d playing", i, s.Path, s.Part, s.Plays, want.path, want.part)
		}
	}
	if parts, err := h.st.LibraryPlayingParts(ctx, string(item.ID)); err != nil || len(parts) != 2 || parts[0].Path != one || parts[1].Path != two {
		t.Errorf("playing files = %+v, %v; want both parts in order", parts, err)
	}
	if page, err := h.st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Problems: true}); err != nil || page.Total != 0 {
		t.Errorf("problems = %+v, %v; want none: neither part is an extra copy", page.Items, err)
	}
	if page, err := h.st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{}); err != nil || len(page.Items) != 1 || page.Items[0].ExtraCopies != 0 {
		t.Errorf("titles = %+v, %v; want one with no extra copy", page.Items, err)
	}
	titles, err := h.st.LibraryTitles(ctx, time.Time{})
	if err != nil || len(titles) != 1 || titles[0].SizeBytes != 50 {
		t.Errorf("titles = %+v, %v; want one title the size of both parts", titles, err)
	}
	length := (&Matcher{Inventory: h.inv}).length(ctx, librarystore.LibraryNeed{ID: string(item.ID), Kind: string(mp.KindMovie)})
	if length != 2*1_320_000*time.Millisecond {
		t.Errorf("length = %v, want both parts' together", length)
	}
	w, err := walkRoot(ctx, h.root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range w.Files {
		if _, ok := f.Sidecars[filepath.Join(h.root, dir, "poster.jpg")]; !ok || !f.Sole {
			t.Errorf("%s: sole=%v sidecars %v, want the folder's poster as its own", f.Rel, f.Sole, f.Sidecars)
		}
	}

	lib := &Library{ItemStore: h.st, Inventory: h.inv}
	detail, err := lib.Item(ctx, string(item.ID))
	if err != nil {
		t.Fatal(err)
	}
	if detail.Name != "Ashgrove (2015)" {
		t.Errorf("name = %q, want the movie's, without a part mark", detail.Name)
	}
	for _, f := range detail.Files {
		if f.Extra || f.Part == 0 {
			t.Errorf("file %s: part %d extra=%v, want a playing part", f.Path, f.Part, f.Extra)
		}
	}

	whole := h.file(dir+"Ashgrove (2015) {tmdb-90000311}.mkv", 40)
	if err := os.Chtimes(filepath.Dir(whole), time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	h.run()
	for _, s := range librarySources(h.item(fileOrigin(h.root, dir+"Ashgrove (2015) {tmdb-90000311}.mkv"))) {
		if s.Plays != (s.Path == whole) {
			t.Errorf("%s plays=%v, want only the whole copy playing", s.Path, s.Plays)
		}
	}
}

// A movie in parts is one title from its first pass, before any catalog names it, whether its
// parts sit side by side or one to a folder ("CD1/", "Disc 2/"): one title for the maintainer to
// answer, named as the movie, whose length is its parts' together. Its parts' shared mark is the
// scanner's own and never listed as an id. A pass over one part's folder keeps it a part.
func TestAMovieInPartsIsOneTitleBeforeAnyCatalog(t *testing.T) {
	for _, c := range []struct {
		name  string
		files []string
	}{
		{"side by side", []string{"Ashgrove (2015)/Ashgrove (2015) - cd1.mkv", "Ashgrove (2015)/Ashgrove (2015) - cd2.mkv"}},
		{"cd folders", []string{"Ashgrove (2015)/CD1/asg-a.avi", "Ashgrove (2015)/CD2/asg-b.avi"}},
		{"disc folders", []string{"Ashgrove (2015)/Disc 1/Ashgrove.avi", "Ashgrove (2015)/Disc 2/Ashgrove.avi"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			h := newHarness(t)
			var paths []string
			for _, f := range c.files {
				paths = append(paths, h.file(f, 10))
			}
			h.run()
			check := func(when string) {
				t.Helper()
				page, err := h.st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{})
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Items) != 1 || page.Items[0].Name != "Ashgrove (2015)" || page.Items[0].Files != 2 {
					t.Fatalf("%s: titles = %+v, want one Ashgrove (2015) of two files", when, page.Items)
				}
				it, err := h.st.LibraryItem(ctx, page.Items[0].ID)
				if err != nil {
					t.Fatal(err)
				}
				if len(it.IDs) != 0 {
					t.Errorf("%s: ids = %+v, want none listed", when, it.IDs)
				}
				parts, err := h.st.LibraryPlayingParts(ctx, it.ID)
				if err != nil || len(parts) != 2 || parts[0].Path != paths[0] || parts[1].Path != paths[1] {
					t.Errorf("%s: playing = %+v, %v; want both parts in order", when, parts, err)
				}
				length := (&Matcher{Inventory: h.inv}).length(ctx, librarystore.LibraryNeed{ID: it.ID, Kind: string(mp.KindMovie)})
				if length != 2*1_320_000*time.Millisecond {
					t.Errorf("%s: length = %v, want both parts' together", when, length)
				}
			}
			check("first pass")
			// A changed second part is written again on a pass over its folder alone.
			if err := os.WriteFile(paths[1], make([]byte, 11), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := h.im.RunFolders(ctx, h.root, []string{filepath.Dir(paths[1])}); err != nil {
				t.Fatal(err)
			}
			check("pass over one part's folder")
		})
	}
}

// Folders are a movie's parts only as a whole set named by their marks alone, each holding one
// video: a show's season folders, a folder that merely starts with a mark, a lone "CD1" or one
// holding two videos keep their videos as they were.
func TestPartFoldersAreAWholeSetOfOneVideoEach(t *testing.T) {
	for _, c := range []struct {
		name  string
		files []string
	}{
		{"a lone part folder", []string{"Ashgrove (2015)/CD1/asg-a.avi", "Ashgrove (2015)/Extras/asg-b.avi"}},
		{"a part missing", []string{"Ashgrove (2015)/CD1/asg-a.avi", "Ashgrove (2015)/CD3/asg-b.avi"}},
		{"words beside the mark", []string{"Ashgrove (2015)/Ashgrove CD1/asg-a.avi", "Ashgrove (2015)/Ashgrove CD2/asg-b.avi"}},
		{"two videos in a part", []string{"Ashgrove (2015)/CD1/asg-a.avi", "Ashgrove (2015)/CD1/asg-c.avi", "Ashgrove (2015)/CD2/asg-b.avi"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			for _, f := range c.files {
				h.file(f, 10)
			}
			w, err := walkRoot(context.Background(), h.root, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range w.Files {
				if f.Part != 0 {
					t.Errorf("%s: part %d, want a whole file", f.Rel, f.Part)
				}
			}
		})
	}
}
