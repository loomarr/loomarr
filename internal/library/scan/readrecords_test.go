package scan

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// On a first import a show's first read asks for its season, and the Enricher fetches it while
// the rest of its headers are read. A read after that describes its episode by the season, rather
// than every episode of the pass being written waiting and then again once the pass is over.
func TestAReadDescribesItsEpisodeByASeasonFetchedSinceTheWalk(t *testing.T) {
	ctx := context.Background()
	src := mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB}
	entry := func(n int, title string) mp.Record {
		return mp.Record{
			Source: src, Kind: mp.KindEpisode, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000971" + string(rune('0'+n))}},
			Numbering: &mp.Numbering{Season: 1, Episode: n}, Descriptive: &mp.Descriptive{Title: title, Overview: title + "."},
		}
	}
	catalog := &fakeCatalog{records: map[string]mp.Record{
		"90000971": {Source: src, Kind: mp.KindSeries, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000971"}},
			Descriptive: &mp.Descriptive{Title: "Tern Point", Orders: []mp.EpisodeOrder{}, Seasons: []mp.SeasonSize{{Number: 1, Episodes: 2}}}},
		"90000971/1": {Source: src, Kind: mp.KindSeason, Children: []mp.Record{entry(1, "Low Water"), entry(2, "High Water")}},
	}}
	h := newHarness(t)
	h.im.Catalog = h.st
	var (
		mu  sync.Mutex
		now = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	)
	h.im.Now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	e := &Enricher{Store: h.st, Provider: catalog, Rewrite: func(string, ...string) {}}
	// The show's record is cached, as a first import's lookups have fetched it by the time its
	// episodes are read; its season is not.
	dir := "Tern Point {tmdb-90000971}/Season 01/"
	h.file(dir+"Tern Point - S01E01.mkv", 10)
	h.run()
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	h.file(dir+"Tern Point - S01E02.mkv", 10)
	// The pass's walk writes the first episode again, now that its show is cached, and asks for
	// the season; the second's read waits for that, and the Enricher fetches it meanwhile.
	season := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "season", ExternalID: "90000971/1"}
	read := h.im.Header
	h.im.Header = func(ctx context.Context, path string) (header.Header, error) {
		if strings.HasSuffix(path, "S01E02.mkv") {
			fetched := false
			for deadline := time.Now().Add(5 * time.Second); !fetched && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
				if err := e.Tick(ctx); err != nil {
					t.Error(err)
				}
				_, fetched, _ = h.st.LibraryCatalogRecord(ctx, season)
			}
			if !fetched {
				t.Error("the season was not fetched before the second read")
			}
			mu.Lock()
			now = now.Add(recheckWaiting)
			mu.Unlock()
		}
		return read(ctx, path)
	}
	h.run()
	o := fileOrigin(h.root, dir+"Tern Point - S01E02.mkv")
	facts := h.origin(h.item(o), o).Observation.Facts
	if facts.Name != "High Water" || facts.Overview != "High Water." {
		t.Errorf("second episode = %q (%q), want the season's \"High Water\" as its header was read", facts.Name, facts.Overview)
	}
	rows, err := h.st.LibraryFiles(ctx, h.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if strings.HasSuffix(r.Path, "S01E02.mkv") && strings.HasPrefix(r.Identity, "waiting:") {
			t.Errorf("the second episode's row is still waiting: it would be written again after its pass")
		}
	}
}
