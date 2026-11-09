package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/tmdb"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
)

func TestSearchQueryReadsTheTitleAndYearFromAName(t *testing.T) {
	for _, tc := range []struct {
		name  string
		known int
		title string
		year  int
	}{
		{"Mystery.Film.2002.1080p.BluRay.x264-GRP", 0, "Mystery Film", 2002},
		{"Mystery Film (2002)", 0, "Mystery Film", 2002},
		{"Mystery Film (2002) [1080p]", 0, "Mystery Film", 2002},
		{"Harbor Lights 2049 (2017)", 0, "Harbor Lights 2049", 2017},
		{"1999 A Quiet Winter 1968", 0, "1999 A Quiet Winter", 1968},
		{"1947", 0, "1947", 0},
		{"Kite-Kid.Over.The.Kite-Line.2018.2160p.WEB-DL", 0, "Kite-Kid Over The Kite-Line", 2018},
		{"Home_Movies_S01", 0, "Home Movies S01", 0},
		{"Mystery Film", 2002, "Mystery Film", 2002},
		{"Mystery Film 1999", 2002, "Mystery Film", 2002},
		{"Mystery Film {tmdb-90000201}", 0, "Mystery Film", 0},
	} {
		title, year := searchQuery(tc.name, tc.known)
		if title != tc.title || year != tc.year {
			t.Errorf("searchQuery(%q, %d) = %q, %d; want %q, %d", tc.name, tc.known, title, year, tc.title, tc.year)
		}
	}
}

func TestCandidateAgreement(t *testing.T) {
	if !sameName("Harbor's Twelve", "Harbors Twelve") || !sameName("Lantern & Kettle", "lantern and kettle") || sameName("Comet", "Comets") {
		t.Error("names compare by their words, any case and punctuation")
	}
	if !sameYear(2002, 2001) || sameYear(2002, 2004) || sameYear(0, 0) {
		t.Error("years agree a year apart at most, and an unknown year never agrees")
	}
	if !sameLength(95*time.Minute, 91*time.Minute) || !sameLength(180*time.Minute, 168*time.Minute) ||
		sameLength(95*time.Minute, 22*time.Minute) || sameLength(0, 22*time.Minute) {
		t.Error("lengths agree within five minutes or 8%, and an unknown length never agrees")
	}
}

// fakeSearch answers every search with hits, counting the searches.
type fakeSearch struct {
	hits  []SearchHit
	err   error
	terms []string
}

func (f *fakeSearch) search(_ context.Context, term string, _ int) ([]SearchHit, error) {
	f.terms = append(f.terms, term)
	return f.hits, f.err
}

// A title nothing grounds is searched for by its file name; its candidates are kept best first,
// with whether the name, year and length agree; it is not searched again until its name changes.
// Linking it grounds it and takes it off Needs you, taking the link back puts it back, and
// ignoring it takes it off the Library too.
func TestMatcherFindsCandidatesAndAnAnswerGroundsTheTitle(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Answers = h.st
	h.file("Mystery.Film.2002.1080p.BluRay.x264-GRP/Mystery.Film.2002.1080p.BluRay.x264-GRP.mkv", 10)
	h.file("Grounded (2010) {tmdb-90000300}/Grounded (2010) {tmdb-90000300}.mkv", 10)
	h.run()

	lib := &Library{NeedStore: h.st, ItemStore: h.st, Keys: fakeCatalogKeys}
	needs, err := lib.Needs(ctx)
	if err != nil || len(needs) != 1 || needs[0].Kind != "movie" {
		t.Fatalf("needs = %+v, %v; want the movie nothing grounds", needs, err)
	}

	search := &fakeSearch{hits: []SearchHit{
		{Kind: "series", ID: tmdbID("90000301"), Name: "Mystery Film", Year: 2002},
		{Kind: "movie", ID: tmdbID("90000302"), Name: "Mystery Films", Year: 2002},
		{Kind: "movie", ID: tmdbID("90000303"), Name: "Mystery Film", Year: 2002, Overview: "A mystery."},
	}}
	catalog := &fakeCatalog{records: map[string]mp.Record{
		"90000302": {Kind: mp.KindMovie, Descriptive: &mp.Descriptive{Runtime: 95 * time.Minute}},
		"90000303": {Kind: mp.KindMovie, Descriptive: &mp.Descriptive{Runtime: 22 * time.Minute},
			Artwork: []mp.ArtworkRef{{Kind: "poster", Ref: "https://image.invalid/90000303.jpg"}}},
	}}
	m := &Matcher{Store: h.st, Search: search.search, Provider: catalog, Inventory: h.inv}
	if err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(search.terms) != 1 || search.terms[0] != "Mystery Film" {
		t.Fatalf("searched %v", search.terms)
	}
	needs, _ = lib.Needs(ctx)
	got := needs[0].Candidates
	want := []store.LibraryCandidate{
		{Kind: "movie", ID: tmdbLink("90000303"), Name: "Mystery Film", Year: 2002, Overview: "A mystery.",
			Poster: "https://image.invalid/90000303.jpg", NameMatch: true, YearMatch: true, LengthMatch: true},
		{Kind: "movie", ID: tmdbLink("90000302"), Name: "Mystery Films", Year: 2002, YearMatch: true},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("candidates = %+v\nwant %+v", got, want)
	}

	// Searched: the next tick leaves it be.
	if err := m.Tick(ctx); err != nil || len(search.terms) != 1 {
		t.Fatalf("second tick searched %v, %v", search.terms, err)
	}

	if err := lib.Link(ctx, needs[0].ID, tmdbLink("90000303")); err != nil {
		t.Fatal(err)
	}
	h.run()
	if needs, _ := lib.Needs(ctx); len(needs) != 0 {
		t.Fatalf("needs after the link = %+v", needs)
	}
	origin := fileOrigin(h.root, "Mystery.Film.2002.1080p.BluRay.x264-GRP/Mystery.Film.2002.1080p.BluRay.x264-GRP.mkv")
	item := h.item(origin)
	if !hasID(item.ExternalIDs, "tmdb", "90000303") {
		t.Fatalf("linked item ids = %+v", item.ExternalIDs)
	}

	if err := lib.Unanswer(ctx, string(item.ID)); err != nil {
		t.Fatal(err)
	}
	h.run()
	if needs, _ := lib.Needs(ctx); len(needs) != 1 {
		t.Fatalf("needs after taking the link back = %+v", needs)
	}

	if err := lib.Ignore(ctx, string(item.ID)); err != nil {
		t.Fatal(err)
	}
	h.run()
	page, err := lib.Items(ctx, store.LibraryItemQuery{})
	if err != nil || page.Total != 1 || page.Items[0].Name == needs[0].Name {
		t.Fatalf("library after the ignore = %+v, %v", page, err)
	}
	if needs, _ := lib.Needs(ctx); len(needs) != 0 {
		t.Fatalf("needs after the ignore = %+v", needs)
	}

	// The grounded title cannot be answered without a scan origin of its own kind.
	if err := lib.Link(ctx, "no-such-item", tmdbLink("90000303")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown item link err = %v", err)
	}
	// A link names an id the catalog fetches for the title's kind: anything else would leave the
	// title answered with no details to show.
	for _, id := range []store.LibraryID{{Namespace: "tvdb", Value: "90000304"}, {Namespace: "tmdb"}, {Namespace: "", Value: "90000304"}} {
		if err := lib.Link(ctx, string(item.ID), id); !errors.Is(err, ErrNotCatalogID) {
			t.Fatalf("link to %+v err = %v, want ErrNotCatalogID", id, err)
		}
	}
}

// tmdbID is a TMDB id as a search hit names it; tmdbLink, as an answer does.
func tmdbID(v string) mp.ExternalID { return mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: v} }

func tmdbLink(v string) store.LibraryID { return store.LibraryID{Namespace: "tmdb", Value: v} }

// A movie nothing grounds is matched by its search when exactly one result agrees on its name,
// year and length (decision 56): it leaves Needs you, decided by the search. Two agreeing results
// are a guess, and wait for the maintainer (58). The maintainer's answer replaces the search's.
func TestMatcherSettlesAMovieOnlyWhenOneResultAgrees(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		hits    []SearchHit
		settled string
	}{
		{"one agrees", []SearchHit{
			{Kind: "movie", ID: tmdbID("90000311"), Name: "Mystery Film", Year: 2002},
			{Kind: "movie", ID: tmdbID("90000312"), Name: "Mystery Film", Year: 1971},
		}, "90000311"},
		{"two agree", []SearchHit{
			{Kind: "movie", ID: tmdbID("90000311"), Name: "Mystery Film", Year: 2002},
			{Kind: "movie", ID: tmdbID("90000313"), Name: "Mystery Film", Year: 2003},
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.im.Answers = h.st
			h.file("Mystery Film (2002)/Mystery Film (2002).mkv", 10)
			h.run()
			lib := &Library{NeedStore: h.st, ItemStore: h.st, Inventory: h.inv, Keys: fakeCatalogKeys}
			// Every candidate runs 22 minutes, as long as the harness's file.
			catalog := &fakeCatalog{records: map[string]mp.Record{}}
			for _, hit := range tc.hits {
				catalog.records[hit.ID.Value] = mp.Record{Kind: mp.KindMovie, Descriptive: &mp.Descriptive{Runtime: 22 * time.Minute}}
			}
			search := &fakeSearch{hits: tc.hits}
			m := &Matcher{Store: h.st, Search: search.search, Provider: catalog, Inventory: h.inv, Match: lib.MatchBySearch}
			if err := m.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			h.run()
			origin := fileOrigin(h.root, "Mystery Film (2002)/Mystery Film (2002).mkv")
			item := h.item(origin)
			needs, _ := lib.Needs(ctx)
			if tc.settled == "" {
				if len(needs) != 1 || hasID(item.ExternalIDs, "tmdb", "90000311") {
					t.Fatalf("needs %d, ids %+v; want it left to the maintainer", len(needs), item.ExternalIDs)
				}
				return
			}
			listed, err := h.st.LibraryItem(ctx, string(item.ID))
			if err != nil {
				t.Fatal(err)
			}
			if len(needs) != 0 || !hasID(item.ExternalIDs, "tmdb", tc.settled) || DecidedBy(listed.Match) != DecidedBySearch {
				t.Fatalf("needs %d, ids %+v, decided by %q; want it matched by the search",
					len(needs), item.ExternalIDs, DecidedBy(listed.Match))
			}

			if err := lib.Link(ctx, string(item.ID), tmdbLink("90000312")); err != nil {
				t.Fatal(err)
			}
			h.run()
			item = h.item(origin)
			listed, _ = h.st.LibraryItem(ctx, string(item.ID))
			if !hasID(item.ExternalIDs, "tmdb", "90000312") || DecidedBy(listed.Match) != DecidedByYou {
				t.Fatalf("ids %+v, decided by %q; want the maintainer's answer", item.ExternalIDs, DecidedBy(listed.Match))
			}
		})
	}
}

// A settled title whose answer fails to save is searched and settled again on the next tick,
// rather than left in Needs you as searched.
func TestMatcherSettlesAgainAfterAFailedAnswer(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Answers = h.st
	h.file("Mystery Film (2002)/Mystery Film (2002).mkv", 10)
	h.run()
	catalog := &fakeCatalog{records: map[string]mp.Record{
		"90000311": {Kind: mp.KindMovie, Descriptive: &mp.Descriptive{Runtime: 22 * time.Minute}},
	}}
	search := &fakeSearch{hits: []SearchHit{{Kind: "movie", ID: tmdbID("90000311"), Name: "Mystery Film", Year: 2002}}}
	var answered []string
	fail := true
	m := &Matcher{Store: h.st, Search: search.search, Provider: catalog, Inventory: h.inv,
		Match: func(_ context.Context, itemID string, _ mp.ExternalID) error {
			if fail {
				return errors.New("database is locked")
			}
			answered = append(answered, itemID)
			return nil
		}}
	if err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	fail = false
	if err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(answered) != 1 || len(search.terms) != 2 {
		t.Fatalf("answered %v after %d searches; want it settled by the second", answered, len(search.terms))
	}
}

// Linking a title to one already in the library makes its file a copy of that title; taking the
// link back, by the id the maintainer answered, finds the file there and splits it out again.
func TestTakingBackALinkSplitsTheCopyOut(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Answers = h.st
	h.file("Grounded (2010) {tmdb-90000300}/Grounded (2010) {tmdb-90000300}.mkv", 10)
	h.file("Mystery Film (2002)/Mystery Film (2002).mkv", 10)
	h.run()
	lib := &Library{NeedStore: h.st, ItemStore: h.st, Inventory: h.inv, Keys: fakeCatalogKeys}
	needs, err := lib.Needs(ctx)
	if err != nil || len(needs) != 1 {
		t.Fatalf("needs = %+v, %v", needs, err)
	}
	answered := needs[0].ID

	if err := lib.Link(ctx, answered, tmdbLink("90000300")); err != nil {
		t.Fatal(err)
	}
	h.run()
	page, _ := lib.Items(ctx, store.LibraryItemQuery{})
	if page.Total != 1 || page.Items[0].Files != 2 {
		t.Fatalf("after the link = %+v, want one title with two copies", page)
	}

	if err := lib.Unanswer(ctx, answered); err != nil {
		t.Fatal(err)
	}
	h.run()
	page, _ = lib.Items(ctx, store.LibraryItemQuery{})
	if page.Total != 2 {
		t.Fatalf("after taking it back = %+v, want the copy split out", page)
	}
	if needs, _ := lib.Needs(ctx); len(needs) != 1 {
		t.Fatalf("needs after taking it back = %+v", needs)
	}
}

// A catalog with no key searches nothing and marks nothing searched; it is searched once the key
// is set.
func TestMatcherWaitsForTheCatalogKey(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.file("Mystery Film (2002)/Mystery Film (2002).mkv", 10)
	h.run()
	search := &fakeSearch{err: tmdb.ErrAPIKeyRequired}
	m := &Matcher{Store: h.st, Search: search.search}
	if err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	needs, _ := h.st.LibraryNeeds(ctx)
	if len(needs) != 1 || needs[0].Searched != "" {
		t.Fatalf("needs = %+v, want it unsearched", needs)
	}
	search.err = nil
	if err := m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if needs, _ := h.st.LibraryNeeds(ctx); needs[0].Searched != "Mystery Film|2002" {
		t.Fatalf("searched = %q", needs[0].Searched)
	}
}

// Decision 57: a show no arr or name identifies is matched by its search when exactly one result
// fits its name, its year when the folder names one, and every season and episode on disk. A
// show TMDB lays out differently from the files, or two that fit, wait for the maintainer.
func TestMatcherSettlesASeriesOnlyWhenOneResultHoldsItsEpisodes(t *testing.T) {
	ctx := context.Background()
	big := []mp.SeasonSize{{Number: 0, Episodes: 3}, {Number: 1, Episodes: 10}, {Number: 2, Episodes: 8}}
	small := []mp.SeasonSize{{Number: 1, Episodes: 6}}
	oneLongSeason := []mp.SeasonSize{{Number: 1, Episodes: 22}}
	for _, tc := range []struct {
		name    string
		folder  string
		layouts map[string][]mp.SeasonSize
		settled string
	}{
		{"one holds every episode", "Harbour Lights (1999)",
			map[string][]mp.SeasonSize{"90000411": big, "90000412": small}, "90000411"},
		{"no year in the folder", "Harbour Lights",
			map[string][]mp.SeasonSize{"90000411": big, "90000412": small}, "90000411"},
		{"laid out differently", "Harbour Lights (1999)",
			map[string][]mp.SeasonSize{"90000411": oneLongSeason, "90000412": small}, ""},
		{"two hold them", "Harbour Lights",
			map[string][]mp.SeasonSize{"90000411": big, "90000412": big}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.im.Answers = h.st
			h.file(tc.folder+"/Season 01/Harbour Lights S01E01.mkv", 10)
			h.file(tc.folder+"/Season 01/Harbour Lights S01E07.mkv", 10)
			h.file(tc.folder+"/Season 02/Harbour Lights S02E03-E04.mkv", 10)
			h.file(tc.folder+"/Specials/Harbour Lights S00E05.mkv", 10)
			h.run()
			lib := &Library{NeedStore: h.st, ItemStore: h.st, Inventory: h.inv, Keys: fakeCatalogKeys}
			hits := []SearchHit{
				{Kind: "series", ID: tmdbID("90000411"), Name: "Harbour Lights", Year: 1999},
				{Kind: "series", ID: tmdbID("90000412"), Name: "Harbour Lights", Year: 2000},
			}
			catalog := &fakeCatalog{records: map[string]mp.Record{}}
			for id, layout := range tc.layouts {
				catalog.records[id] = mp.Record{Kind: mp.KindSeries, Descriptive: &mp.Descriptive{Seasons: layout}}
			}
			search := &fakeSearch{hits: hits}
			m := &Matcher{
				Store: h.st, Search: search.search, Provider: catalog, Inventory: h.inv,
				Match: lib.MatchBySearch, Episodes: lib.EpisodesOnDisk,
			}
			if err := m.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			h.run()
			series := h.item(seriesOrigin(h.root, tc.folder))
			needs, _ := lib.Needs(ctx)
			if tc.settled == "" {
				if len(needs) != 1 || hasID(series.ExternalIDs, "tmdb", "90000411") {
					t.Fatalf("needs %d, ids %+v; want it left to the maintainer", len(needs), series.ExternalIDs)
				}
				return
			}
			listed, err := h.st.LibraryItem(ctx, string(series.ID))
			if err != nil {
				t.Fatal(err)
			}
			if len(needs) != 0 || !hasID(series.ExternalIDs, "tmdb", tc.settled) || DecidedBy(listed.Match) != DecidedBySearch {
				t.Fatalf("needs %d, ids %+v, decided by %q; want it matched by the search",
					len(needs), series.ExternalIDs, DecidedBy(listed.Match))
			}
		})
	}
}

// A series is searched again when its episodes change: a new season on disk can settle it.
func TestEvidenceKeyFollowsTheEpisodes(t *testing.T) {
	a := evidence{episodes: map[int]int{2: 4, 1: 7}}.key()
	b := evidence{episodes: map[int]int{1: 7, 2: 4, 3: 1}}.key()
	if a != "|episodes 1:7 2:4" || a == b {
		t.Fatalf("keys %q and %q; want a stable key that changes with the episodes", a, b)
	}
	if got := (evidence{length: time.Minute}).key(); got != "|length" {
		t.Fatalf("movie key = %q, want the key movies were already searched with", got)
	}
}
