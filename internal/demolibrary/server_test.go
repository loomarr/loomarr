package demolibrary_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/library"
)

// The stand-in is only useful if Loomarr's REAL media-server client reads it correctly, so every
// test here drives internal/library against it rather than asserting on raw JSON. Since #2151 step
// 16 Loomarr reads the media server only for the cutover's id map (ServerFiles), its folders and
// Live TV, so those are what these tests drive.
func demoClient(t *testing.T) (*library.Client, demolibrary.Layout) {
	t.Helper()
	l := demolibrary.Layout{Dir: t.TempDir()}
	srv := httptest.NewServer(demolibrary.Server{Layout: l, Library: demolibrary.Invented(), Token: "demo-token"})
	t.Cleanup(srv.Close)
	return library.New(library.Emby, srv.URL, "demo-token", "demo-test"), l
}

// externalID is the value an item carries in one provider namespace, else 0.
func externalID(ids []inventory.ExternalID, namespace string) int {
	for _, id := range ids {
		if id.Namespace == namespace {
			n, _ := strconv.Atoi(id.Value)
			return n
		}
	}
	return 0
}

// The id map reads every film and every episode, each episode under its show.
func TestStandInServesTheWholeCatalogueToTheIDMap(t *testing.T) {
	c, _ := demoClient(t)
	files, err := c.ServerFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	films, shows := 0, map[string]bool{}
	for _, f := range files {
		switch f.Kind {
		case "movie":
			films++
			if id := externalID(f.ExternalIDs, "tmdb"); id < demolibrary.ProviderIDBase {
				t.Errorf("film %q keyed on tmdb %d, inside the range real titles use", f.Name, id)
			}
		case "episode":
			shows[f.SeriesItemID] = true
			if id := externalID(f.SeriesIDs, "tvdb"); id < demolibrary.ProviderIDBase {
				t.Errorf("episode %q's show keyed on tvdb %d, inside the range real titles use", f.Name, id)
			}
		}
	}
	if films+len(shows) != len(demolibrary.Catalogue) {
		t.Fatalf("id map saw %d films and %d shows, want the %d catalogue titles", films, len(shows), len(demolibrary.Catalogue))
	}
	if len(shows) < 40 || films < 30 {
		t.Fatalf("catalogue = %d series, %d films; #1587 asks for ~40 and ~30", len(shows), films)
	}
}

func TestStandInAnswersEpisodesAndFolders(t *testing.T) {
	ctx := context.Background()
	c, l := demoClient(t)
	files, err := c.ServerFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cartoon := demolibrary.Catalogue[0]
	var eps []library.ServerFile
	for _, f := range files {
		if f.Kind == "episode" && f.SeriesItemID == cartoon.ID() {
			eps = append(eps, f)
		}
	}
	if len(eps) != cartoon.Episodes {
		t.Fatalf("episodes = %d, want %d", len(eps), cartoon.Episodes)
	}
	f, _ := demolibrary.FormatByID(cartoon.Format)
	if eps[0].DurationMillis != int64(f.Duration)*1000 {
		t.Fatalf("episode runtime = %dms, want the generated file's %ds", eps[0].DurationMillis, f.Duration)
	}
	folders, err := c.LibraryFolders(ctx, demolibrary.FillerLibraryName)
	if err != nil || len(folders) != 1 {
		t.Fatalf("filler library folders = %v, %v", folders, err)
	}
	if want, _ := filepath.Abs(filepath.Dir(l.Filler(demolibrary.Fillers[0].ID))); folders[0] != want {
		t.Fatalf("filler folder = %q, want the generated fillers' folder %q", folders[0], want)
	}
}

// The open pack runs through the same stand-in: real episode names and per-file runtimes reach
// the real client, and the filler clips point at the downloaded pack files.
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

	files, err := c.ServerFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var series demolibrary.Title
	for _, ti := range lib.Titles {
		if ti.Kind == demolibrary.Series {
			series = ti
			break
		}
	}
	var eps []library.ServerFile
	for _, f := range files {
		ids := f.ExternalIDs
		if f.Kind == "episode" {
			ids = f.SeriesIDs
		}
		if externalID(ids, "tmdb")+externalID(ids, "tvdb") < demolibrary.OpenProviderIDBase {
			t.Errorf("%q keyed below the open pack's provider range", f.Name)
		}
		if f.Kind == "episode" && f.SeriesItemID == series.ID() {
			eps = append(eps, f)
		}
	}
	want := lib.EpisodesOf(series)
	if len(want) < 2 || len(eps) != len(want) {
		t.Fatalf("episodes of %s = %d, want the pack's %d (and several)", series.Name, len(eps), len(want))
	}
	for i, e := range eps {
		shape, _ := lib.Shape(want[i].ID)
		if e.Name != want[i].Name || e.DurationMillis != int64(shape.Duration)*1000 {
			t.Errorf("episode %d = %q %dms, want %q %ds", i+1, e.Name, e.DurationMillis, want[i].Name, shape.Duration)
		}
	}

	folders, err := c.LibraryFolders(ctx, demolibrary.FillerLibraryName)
	if err != nil || len(folders) != 1 {
		t.Fatalf("filler library folders = %v, %v", folders, err)
	}
	if want, _ := filepath.Abs(filepath.Dir(l.PackFiller(lib.Fillers[0].ID))); folders[0] != want {
		t.Fatalf("filler folder = %q, want the pack download folder %q", folders[0], want)
	}
}

func TestStandInRefusesAWrongToken(t *testing.T) {
	l := demolibrary.Layout{Dir: t.TempDir()}
	srv := httptest.NewServer(demolibrary.Server{Layout: l, Library: demolibrary.Invented(), Token: "demo-token"})
	t.Cleanup(srv.Close)
	c := library.New(library.Emby, srv.URL, "wrong", "demo-test")
	if _, err := c.ServerFiles(context.Background()); err == nil {
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
