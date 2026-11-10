package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// fakeCatalog answers fetches from a table; an unknown key is not found, and err fails every fetch.
type fakeCatalog struct {
	records map[string]mp.Record
	err     error
	fetched []string
}

func (f *fakeCatalog) ID() mp.ID { return mp.IDTMDB }

func (f *fakeCatalog) Capabilities() mp.Capabilities {
	return mp.Capabilities{Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB, Resolves: fakeCatalogKeys}
}

// fakeCatalogKeys is what fakeCatalog fetches, declared as TMDB declares it; the harness's
// Importer keys its titles by it.
var fakeCatalogKeys = CatalogKeys{
	{Kind: mp.KindMovie, Namespace: mp.NamespaceTMDB},
	{Kind: mp.KindSeries, Namespace: mp.NamespaceTMDB},
	{Kind: mp.KindEpisode, Namespace: mp.NamespaceTVDB},
	{Kind: mp.KindCollection, Namespace: mp.NamespaceTMDBCollection},
	{Kind: mp.KindSeason, Namespace: mp.NamespaceTMDB},
	{Kind: mp.KindEpisodeOrder, Namespace: mp.NamespaceTMDB},
}

func (f *fakeCatalog) Fetch(_ context.Context, ref mp.Ref) (mp.Record, error) {
	f.fetched = append(f.fetched, string(ref.Kind)+":"+ref.ID.Value)
	if f.err != nil {
		return mp.Record{}, f.err
	}
	rec, ok := f.records[ref.ID.Value]
	if !ok {
		return mp.Record{}, mp.ErrNotFound
	}
	return rec, nil
}

// A title grounded by an id in its name is written from its file name first; the Enricher
// fetches its catalog record, and the title is written again with the catalog's details. A
// record fetched once is not fetched again until the refresh, and a catalog with no key fetches
// nothing and holds nothing back.
func TestEnricherWritesTheCatalogsDetails(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Catalog = h.st
	h.file("Golf (2013) {tmdb-90000201}/Golf (2013) {tmdb-90000201}.mkv", 10)
	h.run()

	page, err := h.st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("items = %+v, %v", page.Items, err)
	}
	if page.Items[0].Name != "Golf (2013)" {
		t.Fatalf("before the fetch the title is named %q, want its file name", page.Items[0].Name)
	}

	catalog := &fakeCatalog{records: map[string]mp.Record{"90000201": {
		Source: mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB},
		Kind:   mp.KindMovie, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000201"}},
		Descriptive: &mp.Descriptive{Title: "Golf Course", Overview: "A round that never ends.", Year: 2013},
	}}}
	var rewrites [][]string
	e := &Enricher{
		Store: h.st, Provider: catalog,
		Rewrite: func(root string, dirs ...string) {
			rewrites = append(rewrites, dirs)
			if err := h.im.RunFolders(ctx, root, dirs); err != nil {
				t.Fatal(err)
			}
		},
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(catalog.fetched) != 1 || catalog.fetched[0] != "movie:90000201" || len(rewrites) != 1 {
		t.Fatalf("fetched %v, rewrote %v", catalog.fetched, rewrites)
	}
	page, err = h.st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != "Golf Course" {
		t.Fatalf("after the fetch items = %+v, %v", page.Items, err)
	}
	origin := fileOrigin(h.root, "Golf (2013) {tmdb-90000201}/Golf (2013) {tmdb-90000201}.mkv")
	if got := h.origin(h.item(origin), origin).Observation.Facts.Overview; got != "A round that never ends." {
		t.Fatalf("overview = %q", got)
	}

	// Fetched: nothing is due until the refresh.
	if err := e.Tick(ctx); err != nil || len(catalog.fetched) != 1 {
		t.Fatalf("second tick fetched %v, %v", catalog.fetched, err)
	}

	// A new title while TMDB has no key: it is asked for, nothing is held back, and it is fetched
	// once the key is set.
	h.file("Hotel (2014) {tmdb-90000202}/Hotel (2014) {tmdb-90000202}.mkv", 10)
	h.run()
	catalog.err = tmdb.ErrAPIKeyRequired
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	catalog.err = nil
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if last := catalog.fetched[len(catalog.fetched)-1]; last != "movie:90000202" {
		t.Fatalf("fetched %v, want the new title once the key is set", catalog.fetched)
	}

	// A catalog that fails holds that title back, without failing the tick.
	h.file("India (2015) {tmdb-90000203}/India (2015) {tmdb-90000203}.mkv", 10)
	h.run()
	catalog.err = errors.New("tmdb: 503")
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	catalog.err = nil
	before := len(catalog.fetched)
	if err := e.Tick(ctx); err != nil || len(catalog.fetched) != before {
		t.Fatalf("a held-back title was fetched again at once: %v, %v", catalog.fetched[before:], err)
	}
	e.Now = func() time.Time { return time.Now().Add(enrichRetry + time.Minute) }
	if err := e.Tick(ctx); err != nil || len(catalog.fetched) != before+1 {
		t.Fatalf("after the hold the title was not fetched: %v, %v", catalog.fetched[before:], err)
	}
}

// Update details fetches the title's record at once, not on the refresh, and the title takes the
// new details. A catalog that fails says so; one with no key still writes the title again.
func TestUpdateDetailsFetchesTheTitleNow(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Catalog = h.st
	h.file("Golf (2013) {tmdb-90000201}/Golf (2013) {tmdb-90000201}.mkv", 10)
	h.run()
	record := func(title string) mp.Record {
		return mp.Record{
			Source: mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB},
			Kind:   mp.KindMovie, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000201"}},
			Descriptive: &mp.Descriptive{Title: title, Year: 2013},
		}
	}
	catalog := &fakeCatalog{records: map[string]mp.Record{"90000201": record("Golf Course")}}
	rewrite := func(root string, dirs ...string) {
		if err := h.im.RunFolders(ctx, root, dirs); err != nil {
			t.Fatal(err)
		}
	}
	e := &Enricher{Store: h.st, Provider: catalog, Rewrite: rewrite}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	title := func() librarystore.LibraryItemSummary {
		t.Helper()
		page, err := h.st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("items = %+v, %v", page.Items, err)
		}
		return page.Items[0]
	}
	golf := title()
	if golf.Name != "Golf Course" || DecidedBy(golf.Match) != DecidedByFile {
		t.Fatalf("title = %+v, want the catalog's name, decided by the id in its file name", golf)
	}

	scanner := NewScanner(h.im, 0)
	lib := &Library{ItemStore: h.st, NeedStore: h.st, Enricher: e, Scanner: scanner}
	catalog.records["90000201"] = record("Golf Course Redux")
	fetched := len(catalog.fetched)
	if err := lib.UpdateDetails(ctx, golf.ID); err != nil {
		t.Fatal(err)
	}
	if len(catalog.fetched) != fetched+1 {
		t.Fatalf("fetched %v, want the title fetched now", catalog.fetched[fetched:])
	}
	if got := title().Name; got != "Golf Course Redux" {
		t.Fatalf("after Update details the title is %q", got)
	}

	catalog.err = errors.New("tmdb: 503")
	if err := lib.UpdateDetails(ctx, golf.ID); !errors.Is(err, ErrCatalogUnreachable) {
		t.Fatalf("a failing catalog = %v, want ErrCatalogUnreachable", err)
	}
	catalog.err = tmdb.ErrAPIKeyRequired
	if err := lib.UpdateDetails(ctx, golf.ID); err != nil {
		t.Fatalf("no TMDB key = %v, want the title written again from its files", err)
	}
	if err := lib.UpdateDetails(ctx, "90000999"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("an unknown title = %v, want not found", err)
	}
}

// An episode known by its own TVDB id, here from an NFO with no plot and no arr, asks the catalog
// for TMDB's view of it; once fetched, the episode is written again with TMDB's details, which
// the NFO's only fill in for (decision 59).
func TestEnricherDescribesAnEpisodeByItsTVDBID(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Catalog = h.st
	rel := "Golf Show/Season 01/Golf Show S01E02.mkv"
	h.file(rel, 10)
	doc := `<episodedetails><title>The Back Nine</title><season>1</season><episode>2</episode>` +
		`<uniqueid type="tvdb" default="true">90090902</uniqueid></episodedetails>`
	if err := os.WriteFile(filepath.Join(h.root, "Golf Show", "Season 01", "Golf Show S01E02.nfo"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	h.run()
	origin := fileOrigin(h.root, rel)
	if got := h.origin(h.item(origin), origin).Observation.Facts.Overview; got != "" {
		t.Fatalf("before the fetch overview = %q, want none", got)
	}

	catalog := &fakeCatalog{records: map[string]mp.Record{"90090902": {
		Source: mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB},
		Kind:   mp.KindEpisode,
		IDs: []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90090902"},
			{Namespace: mp.NamespaceTMDB, Value: "90090992"}},
		Descriptive: &mp.Descriptive{Title: "Back Nine", Overview: "The course turns against them."},
	}}}
	e := &Enricher{
		Store: h.st, Provider: catalog,
		Rewrite: func(root string, dirs ...string) {
			if err := h.im.RunFolders(ctx, root, dirs); err != nil {
				t.Fatal(err)
			}
		},
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(catalog.fetched) != 1 || catalog.fetched[0] != "episode:90090902" {
		t.Fatalf("fetched %v, want the episode by its TVDB id", catalog.fetched)
	}
	facts := h.origin(h.item(origin), origin).Observation.Facts
	if facts.Overview != "The course turns against them." || facts.Name != "Back Nine" {
		t.Fatalf("after the fetch name %q, overview %q; want TMDB's", facts.Name, facts.Overview)
	}
	if facts.SeasonNumber != 1 || facts.EpisodeNumber != 2 {
		t.Fatalf("numbering = S%dE%d, want the file's S1E2 (TMDB's record carries none)", facts.SeasonNumber, facts.EpisodeNumber)
	}
}

// An episode with no TVDB id takes TMDB's entry for its season and number, but only when TMDB
// numbers the series as the disk does: no other ordering, and each season on disk ends at TMDB's
// count for it (decision 53). Otherwise its season is never asked for, and it keeps its file's
// details.
func TestEnricherNumbersAnEpisodeByItsSeasonOnlyWhenTMDBAgrees(t *testing.T) {
	src := mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB}
	series := func(orders, seasonOne int) mp.Record {
		known := []mp.EpisodeOrder{}
		for range orders {
			known = append(known, mp.EpisodeOrder{ID: "90000799a", Name: "DVD Order", Kind: mp.OrderDVD})
		}
		return mp.Record{
			Source: src, Kind: mp.KindSeries, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000701"}},
			Descriptive: &mp.Descriptive{
				Title: "Golf Show", Orders: known,
				Seasons: []mp.SeasonSize{{Number: 0, Episodes: 2}, {Number: 1, Episodes: seasonOne}},
			},
		}
	}
	entry := func(n int, title string) mp.Record {
		return mp.Record{
			Source: src, Kind: mp.KindEpisode, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "9000079" + strconv.Itoa(n)}},
			Numbering: &mp.Numbering{Season: 1, Episode: n}, Descriptive: &mp.Descriptive{Title: title, Overview: title + "."},
		}
	}
	season := mp.Record{Source: src, Kind: mp.KindSeason, Children: []mp.Record{
		entry(1, "Tee Off"), entry(2, "The Back Nine"), entry(3, "Sudden Death"),
	}}
	for _, tc := range []struct {
		name   string
		series mp.Record
		want   string
	}{
		{"TMDB numbers it as the disk does", series(0, 3), "The Back Nine"},
		{"TMDB knows another ordering", series(1, 3), "Golf Show S01E02"},
		{"a season on disk is not TMDB's size", series(0, 4), "Golf Show S01E02"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h := newHarness(t)
			reads := &countingCatalog{CatalogStore: h.st, reads: map[string]int{}}
			h.im.Catalog = reads
			for n := 1; n <= 3; n++ {
				h.file(fmt.Sprintf("Golf Show {tmdb-90000701}/Season 01/Golf Show S01E0%d.mkv", n), 10)
			}
			h.run()
			catalog := &fakeCatalog{records: map[string]mp.Record{"90000701": tc.series, "90000701/1": season}}
			e := &Enricher{
				Store: h.st, Provider: catalog,
				Rewrite: func(root string, dirs ...string) {
					if err := h.im.RunFolders(ctx, root, dirs); err != nil {
						t.Fatal(err)
					}
				},
			}
			// The series' record first; with it, the episodes ask for their season's.
			for range 2 {
				if err := e.Tick(ctx); err != nil {
					t.Fatal(err)
				}
			}
			askedSeason := slices.Contains(catalog.fetched, "season:90000701/1")
			if numbered := tc.want != "Golf Show S01E02"; askedSeason != numbered {
				t.Fatalf("fetched %v; want the season asked for: %v", catalog.fetched, numbered)
			}
			origin := fileOrigin(h.root, "Golf Show {tmdb-90000701}/Season 01/Golf Show S01E02.mkv")
			facts := h.origin(h.item(origin), origin).Observation.Facts
			if facts.Name != tc.want || facts.SeasonNumber != 1 || facts.EpisodeNumber != 2 {
				t.Fatalf("episode = %q S%dE%d, want %q S1E2", facts.Name, facts.SeasonNumber, facts.EpisodeNumber, tc.want)
			}
			// Each pass that wrote the episodes read their season once, not once an episode: before
			// it was fetched, and again after.
			if got := reads.reads["tmdb:season:90000701/1"]; askedSeason && got != 2 {
				t.Errorf("the season record was read %d times over two passes of three episodes, want once a pass", got)
			}
		})
	}
}

// A show TMDB numbers in more than one order is numbered as the household said its files follow
// (decision 53): TMDB's own seasons, one of its other orders, or the files. With no answer, its
// episodes keep their files' names, are marked unnumbered, which puts the show in Needs you with
// how far each season runs on disk, and TMDB's other orders are asked for.
func TestEnricherNumbersAShowByTheOrderTheHouseholdPicked(t *testing.T) {
	src := mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB}
	seriesKey := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "series", ExternalID: "90000701"}
	entry := func(season, n int, title string) mp.Record {
		return mp.Record{
			Source: src, Kind: mp.KindEpisode, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "9000079" + strconv.Itoa(n)}},
			Numbering: &mp.Numbering{Season: season, Episode: n}, Descriptive: &mp.Descriptive{Title: title},
		}
	}
	records := map[string]mp.Record{
		"90000701": {
			Source: src, Kind: mp.KindSeries, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000701"}},
			Descriptive: &mp.Descriptive{
				Title: "Golf Show", Seasons: []mp.SeasonSize{{Number: 1, Episodes: 3}},
				Orders: []mp.EpisodeOrder{{ID: "90000799a", Name: "DVD Order", Kind: mp.OrderDVD, Seasons: 1, Episodes: 3}},
			},
		},
		"90000701/1": {Source: src, Kind: mp.KindSeason, Children: []mp.Record{
			entry(1, 1, "Tee Off"), entry(1, 2, "The Back Nine"), entry(1, 3, "Sudden Death"),
		}},
		"90000799a": {Source: src, Kind: mp.KindEpisodeOrder, Children: []mp.Record{
			entry(1, 1, "Tee Off"), entry(1, 2, "Sudden Death"), entry(1, 3, "The Back Nine"),
		}},
	}
	for _, tc := range []struct {
		name   string
		answer *librarystore.LibraryEpisodeOrder
		want   string
	}{
		{"nobody has answered", nil, "Golf Show S01E02"},
		{"TMDB's own seasons", &librarystore.LibraryEpisodeOrder{Choice: librarystore.OrderByCatalog}, "The Back Nine"},
		{"TMDB's DVD order", &librarystore.LibraryEpisodeOrder{Choice: librarystore.OrderByOrder, OrderID: "90000799a"}, "Sudden Death"},
		{"the files", &librarystore.LibraryEpisodeOrder{Choice: librarystore.OrderByFiles}, "Golf Show S01E02"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h := newHarness(t)
			h.im.Catalog, h.im.Answers = h.st, h.st
			if tc.answer != nil {
				tc.answer.Series = seriesKey
				if err := h.st.SaveLibraryEpisodeOrder(ctx, *tc.answer); err != nil {
					t.Fatal(err)
				}
			}
			for n := 1; n <= 3; n++ {
				h.file(fmt.Sprintf("Golf Show {tmdb-90000701}/Season 01/Golf Show S01E0%d.mkv", n), 10)
			}
			h.run()
			catalog := &fakeCatalog{records: records}
			e := &Enricher{
				Store: h.st, Provider: catalog,
				Rewrite: func(root string, dirs ...string) {
					if err := h.im.RunFolders(ctx, root, dirs); err != nil {
						t.Fatal(err)
					}
				},
			}
			for range 3 {
				if err := e.Tick(ctx); err != nil {
					t.Fatal(err)
				}
			}
			origin := fileOrigin(h.root, "Golf Show {tmdb-90000701}/Season 01/Golf Show S01E02.mkv")
			facts := h.origin(h.item(origin), origin).Observation.Facts
			if facts.Name != tc.want || facts.SeasonNumber != 1 || facts.EpisodeNumber != 2 {
				t.Fatalf("episode = %q S%dE%d, want %q S1E2 (fetched %v)", facts.Name, facts.SeasonNumber, facts.EpisodeNumber,
					tc.want, catalog.fetched)
			}
			// The show's page says which episodes wait on the answer: all of them, until there is one.
			shows, err := h.st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Kind: "series"})
			if err != nil || len(shows.Items) != 1 {
				t.Fatalf("shows = %+v, %v", shows, err)
			}
			lib := &Library{ItemStore: h.st, Inventory: h.inv}
			detail, err := lib.Item(ctx, shows.Items[0].ID)
			if err != nil || len(detail.Seasons) != 1 || len(detail.Seasons[0].Episodes) != 3 {
				t.Fatalf("detail = %+v, %v", detail.Seasons, err)
			}
			for _, ep := range detail.Seasons[0].Episodes {
				if ep.FromFile != (tc.answer == nil) {
					t.Errorf("episode %d from its file = %v, want %v", ep.Number, ep.FromFile, tc.answer == nil)
				}
			}
			needs, err := h.st.LibraryOrderNeeds(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if tc.answer != nil {
				if len(needs) != 0 {
					t.Fatalf("needs = %+v, want none once answered", needs)
				}
				return
			}
			if len(needs) != 1 || needs[0].Name != "Golf Show" || needs[0].Unnumbered != 3 ||
				!reflect.DeepEqual(needs[0].Last, map[int]int{1: 3}) {
				t.Fatalf("needs = %+v, want Golf Show with its 3 episodes unnumbered, season 1 running to 3", needs)
			}
			if !slices.Contains(catalog.fetched, "episode_order:90000799a") {
				t.Errorf("fetched %v, want TMDB's other order asked for", catalog.fetched)
			}
		})
	}
}

// A series record cached in an older shape is fetched again on the next tick, fresh as it is; a
// movie record of the same age, already in the movie's current shape, is left until its refresh.
func TestEnricherFetchesARecordOfAnOlderShapeAgain(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	series := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "series", ExternalID: "90000701"}
	movie := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000201"}
	if err := h.st.WantLibraryCatalog(ctx, []librarystore.LibraryCatalogKey{series, movie}); err != nil {
		t.Fatal(err)
	}
	for k, version := range map[librarystore.LibraryCatalogKey]int{
		series: catalogVersions[series.Kind] - 1, movie: catalogVersions[movie.Kind],
	} {
		if _, err := h.st.SaveLibraryCatalog(ctx, k, `{"Kind":"`+k.Kind+`"}`, version, "", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	catalog := &fakeCatalog{records: map[string]mp.Record{"90000701": {Kind: mp.KindSeries}}}
	e := &Enricher{Store: h.st, Provider: catalog, Rewrite: func(string, ...string) {}}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(catalog.fetched, []string{"series:90000701"}) {
		t.Fatalf("fetched %v, want only the series record, read in its new shape", catalog.fetched)
	}
	if err := e.Tick(ctx); err != nil || len(catalog.fetched) != 1 {
		t.Fatalf("second tick fetched %v, %v; want nothing once it is current", catalog.fetched, err)
	}
}

// When the household's language changes, a record cached in the old one is fetched again on the
// next tick and saved in the language it was read in, so the tick after leaves it be.
func TestEnricherFetchesARecordInAnotherLanguageAgain(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	movie := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000201"}
	if err := h.st.WantLibraryCatalog(ctx, []librarystore.LibraryCatalogKey{movie}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.SaveLibraryCatalog(ctx, movie, `{"Kind":"movie"}`, catalogVersions[movie.Kind], "en-US", time.Now()); err != nil {
		t.Fatal(err)
	}
	catalog := &fakeCatalog{records: map[string]mp.Record{"90000201": {Kind: mp.KindMovie, Language: "fr-US"}}}
	e := &Enricher{Store: h.st, Provider: catalog, Rewrite: func(string, ...string) {},
		Language: func() string { return "fr-US" }}
	for range 2 {
		if err := e.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(catalog.fetched, []string{"movie:90000201"}) {
		t.Fatalf("fetched %v over two ticks, want the English record once, read again in French", catalog.fetched)
	}
}

// countingCatalog counts the cached records the walk reads, by key.
type countingCatalog struct {
	CatalogStore
	reads map[string]int
}

func (c *countingCatalog) LibraryCatalogRecord(ctx context.Context, key librarystore.LibraryCatalogKey) (string, bool, error) {
	c.reads[key.String()]++
	return c.CatalogStore.LibraryCatalogRecord(ctx, key)
}

// Refresh everything writes every file again on the pass it asks for, so a sidecar added since is
// read, without opening any file's bytes again; every record is due for the enricher. The title
// stays ready throughout.
func TestRefreshEverythingWritesEveryFileAgain(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.file("Golf (2013) {tmdb-90000201}/Golf (2013) {tmdb-90000201}.mkv", 10)
	h.file("Hotel (2014) {tmdb-90000202}/Hotel (2014) {tmdb-90000202}.mkv", 10)
	h.run()
	applied, reads := h.inv.applied.Load(), h.reads.Load()
	if reads != 2 {
		t.Fatalf("header reads = %d, want 2", reads)
	}

	h.run()
	if h.inv.applied.Load() != applied {
		t.Fatal("an unchanged library was written again without being asked")
	}

	lib := &Library{ItemStore: h.st, NeedStore: h.st, Scanner: NewScanner(h.im, 0)}
	if err := lib.RefreshEverything(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lib.Scanner.trigger:
	default:
		t.Fatal("Refresh everything did not ask for a pass")
	}
	h.run()
	if got := h.inv.applied.Load() - applied; got != 2 {
		t.Fatalf("written again = %d, want both files", got)
	}
	if h.reads.Load() != reads {
		t.Fatalf("header reads = %d, want none on a refresh", h.reads.Load()-reads)
	}
	page, err := h.st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("items = %+v, %v", page.Items, err)
	}
	for _, it := range page.Items {
		if it.Pending != 0 {
			t.Errorf("%s stopped being ready after the refresh: %d files pending", it.Name, it.Pending)
		}
	}

	h.enable.Store(false)
	if err := lib.RefreshEverything(ctx); !errors.Is(err, ErrScannerOff) {
		t.Fatalf("scanner off = %v, want ErrScannerOff", err)
	}
}

// tickStore tells each time the Enricher counts what is still due as new: once as a tick's last
// read of the store, and once more in a tick that begins step 3.
type tickStore struct {
	EnrichStore
	counted chan struct{}
}

func (s tickStore) CountDueLibraryCatalog(ctx context.Context, due librarystore.LibraryCatalogDue) (int, error) {
	n, err := s.EnrichStore.CountDueLibraryCatalog(ctx, due)
	s.counted <- struct{}{}
	return n, err
}

// Titles written between ticks are looked up when the Enricher is woken, not at the next tick: an
// hour away here. A woken lookup fetches only what is new; records grown old wait for a tick.
func TestEnricherLooksUpNewTitlesWhenWoken(t *testing.T) {
	h := newHarness(t)
	st := tickStore{EnrichStore: h.st, counted: make(chan struct{}, 16)}
	counts := func(n int, what string) {
		t.Helper()
		for range n {
			select {
			case <-st.counted:
			case <-time.After(10 * time.Second):
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}
	base := time.Now()
	var ahead atomic.Int64
	catalog := &fakeCatalog{records: map[string]mp.Record{
		"90000201": {Kind: mp.KindMovie}, "90000202": {Kind: mp.KindMovie},
	}}
	e := &Enricher{
		Store: st, Provider: catalog, Rewrite: func(string, ...string) {}, Tracker: NewTracker(nil, nil),
		Interval: time.Hour, Now: func() time.Time { return base.Add(time.Duration(ahead.Load())) },
		Sleep: func(context.Context, time.Duration) error { return nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Run(ctx)
	}()
	counts(1, "the first tick, with nothing to look up")

	keys := []librarystore.LibraryCatalogKey{
		{Provider: "tmdb", Kind: "movie", ExternalID: "90000201"},
		{Provider: "tmdb", Kind: "movie", ExternalID: "90000202"},
	}
	if err := h.st.WantLibraryCatalog(ctx, keys); err != nil {
		t.Fatal(err)
	}
	e.Wake()
	counts(2, "the woken lookup of two new titles")
	if len(catalog.fetched) != 2 {
		t.Fatalf("woken, fetched %v; want both new titles", catalog.fetched)
	}

	// Both records are older than the refresh period now, and nothing is new.
	ahead.Store(int64(EnrichRefresh + 24*time.Hour))
	e.Wake()
	counts(1, "the woken lookup with nothing new")
	if len(catalog.fetched) != 2 {
		t.Fatalf("woken with nothing new, fetched %v; want no old record refreshed", catalog.fetched)
	}

	cancel()
	<-done
	// A tick does refresh its share of them, so the records above were due as old.
	if err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(catalog.fetched) != 3 {
		t.Fatalf("after a tick, fetched %v; want one old record refreshed", catalog.fetched)
	}
}

// The titles looked up are written again, with their details, by a folder pass. Until that pass
// has run, step 3 stays, done and naming no title, so the import never reads as finished over
// titles still showing their file names. The tick after the pass ends it.
func TestEnricherKeepsTheStepUntilItsRewritesHaveRun(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Catalog = h.st
	h.file("Golf (2013) {tmdb-90000201}/Golf (2013) {tmdb-90000201}.mkv", 10)
	h.file("Hotel (2014) {tmdb-90000202}/Hotel (2014) {tmdb-90000202}.mkv", 10)
	h.run()

	settled := false
	e := &Enricher{
		Store: h.st, Provider: &fakeCatalog{}, Tracker: h.im.Tracker, Rewrite: func(string, ...string) {},
		Settled: func() bool { return settled },
		Sleep:   func(context.Context, time.Duration) error { return nil },
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got := h.im.Tracker.Snapshot(time.Now())
	if got.Step != StepLookingUp || got.Done != 2 || got.Total != 2 || got.Current != "" {
		t.Fatalf("with the rewrites still to run = %+v, want step 3 at 2 of 2 naming no title", got)
	}

	settled = true
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if h.im.Tracker.Active(StepLookingUp) {
		t.Fatal("step 3 still runs after the rewrites have")
	}
}

// Wake never blocks, with no Run draining it and on a nil Enricher.
func TestEnricherWakeNeverBlocks(t *testing.T) {
	e := &Enricher{}
	for range 3 {
		e.Wake()
	}
	var none *Enricher
	none.Wake()
}
