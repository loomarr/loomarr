package demolibrary_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/library"
)

// The stand-in is only useful if Loomarr's REAL media-server client reads it correctly, so every
// test here drives internal/library against it rather than asserting on raw JSON.
func demoClient(t *testing.T) (*library.Client, demolibrary.Layout) {
	t.Helper()
	l := demolibrary.Layout{Dir: t.TempDir()}
	srv := httptest.NewServer(demolibrary.Server{Layout: l, Token: "demo-token"})
	t.Cleanup(srv.Close)
	return library.New(library.Emby, srv.URL, "demo-token", "demo-test"), l
}

func TestStandInServesTheWholeCatalogueToTheLibraryScan(t *testing.T) {
	c, _ := demoClient(t)
	items, err := c.AllItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(demolibrary.Catalogue) {
		t.Fatalf("scan saw %d items, want the %d catalogue titles", len(items), len(demolibrary.Catalogue))
	}
	series, films := 0, 0
	for _, it := range items {
		switch it.MediaType {
		case library.Series:
			series++
			if it.TVDBID < demolibrary.ProviderIDBase {
				t.Errorf("series %q keyed on tvdb %d, inside the range real titles use", it.Name, it.TVDBID)
			}
		case library.Movie:
			films++
			if it.TMDBID < demolibrary.ProviderIDBase {
				t.Errorf("film %q keyed on tmdb %d, inside the range real titles use", it.Name, it.TMDBID)
			}
		}
	}
	if series < 40 || films < 30 {
		t.Fatalf("catalogue = %d series, %d films; #1587 asks for ~40 and ~30", series, films)
	}
}

func TestStandInAnswersSearchPresenceEpisodesAndPaths(t *testing.T) {
	ctx := context.Background()
	c, l := demoClient(t)

	got, err := c.Search(ctx, "science fiction", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("genre search found nothing")
	}

	cartoon := demolibrary.Catalogue[0]
	id, present, err := c.Lookup(ctx, library.TVDB, strconv.Itoa(cartoon.ProviderID()), library.Series)
	if err != nil || !present || id != cartoon.ID() {
		t.Fatalf("Lookup(%s) = %q, %v, %v; want %q present", cartoon.Key(), id, present, err, cartoon.ID())
	}
	if _, present, _ := c.Lookup(ctx, library.TMDB, "603", library.Movie); present {
		t.Fatal("a real-world tmdb id resolved in the demo library")
	}

	eps, err := c.ListEpisodes(ctx, cartoon.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != cartoon.Episodes {
		t.Fatalf("episodes = %d, want %d", len(eps), cartoon.Episodes)
	}
	f, _ := demolibrary.FormatByID(cartoon.Format)
	if eps[0].DurationMs != int64(f.Duration)*1000 {
		t.Fatalf("episode runtime = %dms, want the generated file's %ds", eps[0].DurationMs, f.Duration)
	}
	path, err := c.ItemPath(ctx, eps[0].LibraryItemID)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.Abs(l.Item(eps[0].LibraryItemID)); path != want {
		t.Fatalf("episode path = %q, want the per-item file %q", path, want)
	}

	folder, err := c.LibraryIDByName(ctx, demolibrary.FillerLibraryName)
	if err != nil || folder == "" {
		t.Fatalf("filler library by name = %q, %v", folder, err)
	}
	clips, err := c.ListFillerClips(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != len(demolibrary.Fillers) {
		t.Fatalf("filler clips = %d, want %d", len(clips), len(demolibrary.Fillers))
	}
}

func TestStandInRefusesAWrongToken(t *testing.T) {
	l := demolibrary.Layout{Dir: t.TempDir()}
	srv := httptest.NewServer(demolibrary.Server{Layout: l, Token: "demo-token"})
	t.Cleanup(srv.Close)
	c := library.New(library.Emby, srv.URL, "wrong", "demo-test")
	if _, err := c.AllItems(context.Background()); err == nil {
		t.Fatal("a wrong token read the library")
	}
	resp, err := http.Get(srv.URL + "/System/Info/Public")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public info = %d, want 200 without a token", resp.StatusCode)
	}
}
