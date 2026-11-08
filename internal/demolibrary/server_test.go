package demolibrary_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/library"
)

// The stand-in is only useful if Loomarr's REAL media-server client reads it correctly, so every
// test here drives internal/library against it rather than asserting on raw JSON.
func demoClient(t *testing.T) (*library.Client, demolibrary.Layout) {
	t.Helper()
	l := demolibrary.Layout{Dir: t.TempDir()}
	srv := httptest.NewServer(demolibrary.Server{Layout: l, Library: demolibrary.Invented(), Token: "demo-token"})
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

// An owned film's approval size is read from the media server's Size on the search it already
// makes (#1885). The stand-in states the generated file's real byte count, and no size at all
// before the file exists — never 0, which the contract reserves for "unknown" by omission.
func TestStandInStatesAFilmsFileSizeOnlyOnceTheFileExists(t *testing.T) {
	ctx := context.Background()
	c, l := demoClient(t)
	var film demolibrary.Title
	for _, title := range demolibrary.Catalogue {
		if title.Kind == demolibrary.Movie {
			film = title
			break
		}
	}

	sizeOf := func() int64 {
		t.Helper()
		got, err := c.Search(ctx, film.Name, 5)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range got {
			if r.LibraryItemID == film.ID() {
				return r.SizeBytes
			}
		}
		t.Fatalf("search for %q did not return the film", film.Name)
		return 0
	}

	if got := sizeOf(); got != 0 {
		t.Fatalf("size before the file exists = %d, want none", got)
	}
	path := l.Item(film.ID())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := sizeOf(); got != 4096 {
		t.Fatalf("size = %d, want the file's 4096 bytes", got)
	}
}

// The open pack runs through the same stand-in: real episode names and per-file runtimes reach
// the real client, every title's overview carries its credit and licence, and the filler clips
// point at the downloaded pack files.
func TestStandInServesTheOpenPack(t *testing.T) {
	ctx := context.Background()
	lib, err := demolibrary.Load(demolibrary.ContentOpen)
	if err != nil {
		t.Fatal(err)
	}
	l := demolibrary.Layout{Dir: t.TempDir()}
	srv := httptest.NewServer(demolibrary.Server{Layout: l, Library: lib, Token: "demo-token"})
	t.Cleanup(srv.Close)
	c := library.New(library.Emby, srv.URL, "demo-token", "demo-test")

	items, err := c.AllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(lib.Titles) {
		t.Fatalf("scan saw %d items, want the pack's %d titles", len(items), len(lib.Titles))
	}
	for _, it := range items {
		if it.TMDBID+it.TVDBID < demolibrary.OpenProviderIDBase {
			t.Errorf("%q keyed below the open pack's provider range", it.Name)
		}
		if !strings.Contains(it.Overview, "Licence: ") {
			t.Errorf("%q overview carries no licence: %q", it.Name, it.Overview)
		}
	}

	var series demolibrary.Title
	for _, ti := range lib.Titles {
		if ti.Kind == demolibrary.Series {
			series = ti
			break
		}
	}
	want := lib.EpisodesOf(series)
	eps, err := c.ListEpisodes(ctx, series.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(want) < 2 || len(eps) != len(want) {
		t.Fatalf("episodes of %s = %d, want the pack's %d (and several)", series.Name, len(eps), len(want))
	}
	for i, e := range eps {
		shape, _ := lib.Shape(want[i].ID)
		if e.Name != want[i].Name || e.DurationMs != int64(shape.Duration)*1000 {
			t.Errorf("episode %d = %q %dms, want %q %ds", i+1, e.Name, e.DurationMs, want[i].Name, shape.Duration)
		}
	}

	folder, err := c.LibraryIDByName(ctx, demolibrary.FillerLibraryName)
	if err != nil {
		t.Fatal(err)
	}
	clips, err := c.ListFillerClips(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != len(lib.Fillers) {
		t.Fatalf("filler clips = %d, want %d", len(clips), len(lib.Fillers))
	}
	if wantPath, _ := filepath.Abs(l.PackFiller(clips[0].LibraryItemID)); clips[0].Path != wantPath {
		t.Fatalf("filler path = %q, want the pack download %q", clips[0].Path, wantPath)
	}
}

func TestStandInRefusesAWrongToken(t *testing.T) {
	l := demolibrary.Layout{Dir: t.TempDir()}
	srv := httptest.NewServer(demolibrary.Server{Layout: l, Library: demolibrary.Invented(), Token: "demo-token"})
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
