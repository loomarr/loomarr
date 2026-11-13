package scan

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

func radarrMovie(tmdb, file string) mp.Record {
	return mp.Record{
		Source: mp.Source{Provider: mp.IDRadarr, Basis: mp.BasisManager}, Kind: mp.KindMovie,
		IDs:   []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: tmdb}},
		Files: []mp.FileRef{{Path: file}},
	}
}

func tmdbItem(tmdb string) *ArrItem {
	return &ArrItem{Authority: "radarr", Ref: mp.Ref{Kind: mp.KindMovie, ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: tmdb}}}
}

// A folder pass reuses the arrs' listing from the last full pass, reading again only the item an
// arr said changed. A change it cannot read alone, or one that names no item, lists the arr again.
func TestFolderPassReusesTheArrListing(t *testing.T) {
	radarr := &fakeLister{records: []mp.Record{radarrMovie("90000701", "/movies/Alpha (2001)/Alpha.mkv")}}
	address := "http://radarr.invalid:7878"
	h := newHarness(t, Manager{Authority: "radarr", Lister: radarr, Fetcher: radarr,
		Address: func() string { return address }})
	ctx := context.Background()
	if err := h.st.RecordLibraryRootDiscovery(ctx, librarystore.LibraryRootDiscovery{
		Found: []librarystore.LibraryRootPath{{Authority: "radarr", AuthorityPath: "/movies", RootPath: h.root}}, Complete: []string{"radarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.file("Alpha (2001)/Alpha.mkv", 10)
	h.run()
	if radarr.lists != 1 {
		t.Fatalf("lists after the full pass = %d", radarr.lists)
	}
	folder := func(dir string) {
		t.Helper()
		if err := h.im.RunFolders(ctx, h.root, []string{filepath.Join(h.root, dir)}); err != nil {
			t.Fatal(err)
		}
	}

	// Radarr imports a movie and says which.
	h.file("Bravo (2002)/Bravo.mkv", 10)
	radarr.records = append(radarr.records, radarrMovie("90000702", "/movies/Bravo (2002)/Bravo.mkv"))
	h.im.ArrChanged(tmdbItem("90000702"))
	folder("Bravo (2002)")
	if radarr.lists != 1 || radarr.fetches != 1 {
		t.Fatalf("lists, fetches = %d, %d; want the one movie read", radarr.lists, radarr.fetches)
	}
	if item := h.item(fileOrigin(h.root, "Bravo (2002)/Bravo.mkv")); !hasID(item.ExternalIDs, "tmdb", "90000702") {
		t.Fatalf("bravo = %+v, want Radarr's id", item.ExternalIDs)
	}

	// A folder change with no arr behind it reads nothing from the arr.
	h.file("Charlie (2003)/Charlie.mkv", 10)
	folder("Charlie (2003)")
	if radarr.lists != 1 || radarr.fetches != 1 {
		t.Fatalf("lists, fetches = %d, %d after a plain folder change", radarr.lists, radarr.fetches)
	}

	// A read that fails, and a change naming no item, list the arr again.
	radarr.fetchErr = errors.New("radarr hiccup")
	h.im.ArrChanged(tmdbItem("90000701"))
	folder("Alpha (2001)")
	if radarr.lists != 2 {
		t.Fatalf("lists after a failed read = %d, want a fresh listing", radarr.lists)
	}
	h.im.ArrChanged(nil)
	folder("Alpha (2001)")
	if radarr.lists != 3 {
		t.Fatalf("lists after an unnamed change = %d", radarr.lists)
	}
	address = "http://radarr-new.invalid:7878"
	folder("Alpha (2001)")
	if radarr.lists != 4 {
		t.Fatalf("lists after the arr moved = %d", radarr.lists)
	}

	// A movie Radarr deleted leaves the index: its file is no longer Radarr's.
	radarr.fetchErr = nil
	radarr.records = radarr.records[:1]
	h.im.ArrChanged(tmdbItem("90000702"))
	folder("Bravo (2002)")
	if _, ok := h.im.index.byPath[filepath.Join(h.root, "Bravo (2002)", "Bravo.mkv")]; ok {
		t.Fatal("a deleted movie stayed in the index")
	}
}
