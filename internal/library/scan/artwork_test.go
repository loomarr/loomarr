package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/localart"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// fakeArtwork keeps each title's image file name per kind; fail makes every import fail and
// change nothing.
type fakeArtwork struct {
	fail bool
	held map[string]map[string]string // kind → item → file name
	refs map[string]string            // item → the last ref it was handed, whole
}

func (f *fakeArtwork) Art(_ context.Context, itemID, kind, ref string) error {
	if f.fail {
		return errors.New("worker down")
	}
	if f.held == nil {
		f.held = map[string]map[string]string{}
	}
	if f.held[kind] == nil {
		f.held[kind] = map[string]string{}
	}
	if ref == "" {
		delete(f.held[kind], itemID)
		return nil
	}
	f.held[kind][itemID] = filepath.Base(ref)
	if f.refs == nil {
		f.refs = map[string]string{}
	}
	f.refs[itemID] = ref
	return nil
}

// art is each listed title's image of one kind, by name.
func (h *harness) art(f *fakeArtwork, kind string) map[string]string {
	h.t.Helper()
	page, err := h.st.ListLibraryItems(context.Background(), librarystore.LibraryItemQuery{})
	if err != nil {
		h.t.Fatal(err)
	}
	out := map[string]string{}
	for _, it := range page.Items {
		out[it.Name] = f.held[kind][it.ID]
	}
	return out
}

// posters is each listed title's poster, by name.
func (h *harness) posters(f *fakeArtwork) map[string]string {
	return h.art(f, mp.KindPoster)
}

// A title's fanart and clear logo are imported beside its poster (#2567); the other kinds the
// sidecars name (banner, clearart, …) are not, since nothing shows them.
func TestImportFanartAndLogoBesideThePoster(t *testing.T) {
	h := newHarness(t)
	art := &fakeArtwork{}
	h.im.Art, h.im.Artwork = localart.New(), art
	h.file("Alpha/Alpha.mkv", 10)
	h.file("Alpha/poster.jpg", 3)
	h.file("Alpha/fanart.jpg", 3)
	h.file("Alpha/clearlogo.png", 3)
	h.file("Alpha/banner.jpg", 3)
	h.run()

	for kind, want := range map[string]string{
		mp.KindPoster: "poster.jpg", mp.KindFanart: "fanart.jpg", mp.KindClearlogo: "clearlogo.png",
	} {
		if got := h.art(art, kind)["Alpha"]; got != want {
			t.Errorf("%s = %q, want %q", kind, got, want)
		}
	}
	if len(art.held[mp.KindBanner]) != 0 {
		t.Errorf("banner imported: %v", art.held[mp.KindBanner])
	}
}

// The scanner imports each title's poster from beside its media: a movie folder's poster, then the
// movie's own when one appears, and a series folder's. A poster deleted is let go. An import that
// fails changes nothing and the pass carries on.
func TestImportPostersFromBesideTheMedia(t *testing.T) {
	h := newHarness(t)
	posters := &fakeArtwork{}
	h.im.Art, h.im.Artwork = localart.New(), posters
	h.file("Alpha/Alpha.mkv", 10)
	h.file("Alpha/poster.jpg", 3)
	h.file("Bravo Show/Season 01/Bravo Show S01E01.mkv", 10)
	h.file("Bravo Show/Poster.PNG", 3)
	h.file("Charlie/Charlie.mkv", 10)
	h.run()

	got := h.posters(posters)
	if got["Alpha"] != "poster.jpg" || got["Bravo Show"] != "Poster.PNG" || got["Charlie"] != "" {
		t.Fatalf("posters = %v", got)
	}

	// A poster added beside the movie is a change to it, and the movie's own beats the folder's.
	alphaOwn := h.file("Alpha/Alpha-poster.jpg", 3)
	h.run()
	if got := h.posters(posters)["Alpha"]; got != "Alpha-poster.jpg" {
		t.Fatalf("after the movie's own poster = %q", got)
	}

	// Removing both lets the poster go.
	for _, p := range []string{alphaOwn, filepath.Join(h.root, "Alpha", "poster.jpg")} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	h.run()
	if got := h.posters(posters)["Alpha"]; got != "" {
		t.Fatalf("after removing the posters = %q, want none", got)
	}

	// A title written while the image service fails keeps what it had; the pass does not fail.
	posters.fail = true
	h.file("Bravo Show/Season 01/Bravo Show S01E02.mkv", 10)
	h.run()
	if got := h.posters(posters)["Bravo Show"]; got != "Poster.PNG" {
		t.Fatalf("failed import = %q, want the poster kept", got)
	}
}

// An episode's still is imported when the episode is written (#4476): the "<name>-thumb" file
// beside it first, then its provider's URL, else none. A still that changes replaces the one the
// episode had, and an episode imports no other kind.
func TestImportEpisodeStills(t *testing.T) {
	sonarr := &fakeLister{}
	h := newHarness(t, Manager{Authority: "sonarr", Lister: sonarr})
	ctx := context.Background()
	if err := h.st.RecordLibraryRootDiscovery(ctx, librarystore.LibraryRootDiscovery{
		Found: []librarystore.LibraryRootPath{{Authority: "sonarr", AuthorityPath: "/tv", RootPath: h.root}}, Complete: []string{"sonarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	art := &fakeArtwork{}
	h.im.Art, h.im.Artwork = localart.New(), art
	const episodes = 120
	rel := func(n int) string { return fmt.Sprintf("Golf Show/Season 01/Golf Show S01E%03d.mkv", n) }
	var children []mp.Record
	for n := 1; n <= episodes; n++ {
		h.file(rel(n), 10)
		ep := mp.Record{
			Source: mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager}, Kind: mp.KindEpisode,
			IDs:       []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: fmt.Sprintf("9009%04d", n)}},
			Numbering: &mp.Numbering{Season: 1, Episode: n},
			Files:     []mp.FileRef{{Path: "/tv/" + rel(n)}},
		}
		// The third has no still anywhere.
		if n != 3 {
			ep.Artwork = []mp.ArtworkRef{{Kind: mp.KindThumb, Ref: fmt.Sprintf("https://image.invalid/e%d.jpg", n)},
				{Kind: mp.KindFanart, Ref: "https://image.invalid/fanart.jpg"}}
		}
		children = append(children, ep)
	}
	sonarr.records = []mp.Record{{
		Source: mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager}, Kind: mp.KindSeries,
		IDs:      []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90081191"}},
		Children: children,
	}}
	ownThumb := h.file("Golf Show/Season 01/Golf Show S01E001-thumb.jpg", 3)
	h.run()

	still := func(n int) string {
		t.Helper()
		id := h.fileRow(filepath.Join(h.root, filepath.FromSlash(rel(n)))).ItemID
		if id == "" {
			t.Fatalf("episode %d not written", n)
		}
		return art.held[mp.KindThumb][id]
	}
	if got := still(1); got != "Golf Show S01E001-thumb.jpg" {
		t.Errorf("episode 1 still = %q, want the file beside it", got)
	}
	if got := still(3); got != "" {
		t.Errorf("episode 3 still = %q, want none", got)
	}
	// Every episode of a long series hands its provider still over in the pass: the image
	// service only records a URL there, and its fetch job pulls the bytes in bounded batches.
	for n := 2; n <= episodes; n++ {
		if want := fmt.Sprintf("e%d.jpg", n); n != 3 && still(n) != want {
			t.Fatalf("episode %d still = %q, want %q", n, still(n), want)
		}
	}
	if len(art.held[mp.KindFanart]) != 0 {
		t.Errorf("an episode's fanart was imported: %v", art.held[mp.KindFanart])
	}

	// The file beside it gone, the episode's still is its provider's again; one added beside
	// another replaces its provider's.
	if err := os.Remove(ownThumb); err != nil {
		t.Fatal(err)
	}
	h.file("Golf Show/Season 01/Golf Show S01E002-thumb.png", 3)
	h.run()
	if got := still(1); got != "e1.jpg" {
		t.Errorf("after removing its file, episode 1 still = %q, want its provider's", got)
	}
	if got := still(2); got != "Golf Show S01E002-thumb.png" {
		t.Errorf("after adding its file, episode 2 still = %q, want the file", got)
	}
}

// An episode an earlier build wrote, which imported no stills, takes its still once write version
// 92 writes it again (#4476): an unchanged file the earlier build wrote is rewritten with no header
// read, and its still is imported then, the file beside it or the one its cached catalogue record
// names. A record cached by the earlier build names its still at TMDB's original size, and it is
// imported at w300. An episode written at the earlier version and read again by it is left alone,
// so the test fails unless writeVersion moved past it.
func TestEpisodesWrittenAtAnOlderVersionTakeTheirStills(t *testing.T) {
	const stillsImported = 92 // the write version that imports episode stills
	ctx := context.Background()
	sonarr := &fakeLister{}
	h := newHarness(t, Manager{Authority: "sonarr", Lister: sonarr})
	if err := h.st.RecordLibraryRootDiscovery(ctx, librarystore.LibraryRootDiscovery{
		Found: []librarystore.LibraryRootPath{{Authority: "sonarr", AuthorityPath: "/tv", RootPath: h.root}}, Complete: []string{"sonarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.im.Art, h.im.Catalog = localart.New(), h.st
	rel := func(n int) string { return fmt.Sprintf("Hotel Show/Season 01/Hotel Show S01E%02d.mkv", n) }
	var children []mp.Record
	for n := 1; n <= 2; n++ {
		h.file(rel(n), 10)
		children = append(children, mp.Record{
			Source: mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager}, Kind: mp.KindEpisode,
			IDs:       []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: fmt.Sprintf("9009300%d", n)}},
			Numbering: &mp.Numbering{Season: 1, Episode: n},
			Files:     []mp.FileRef{{Path: "/tv/" + rel(n)}},
		})
	}
	sonarr.records = []mp.Record{{
		Source: mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager}, Kind: mp.KindSeries,
		IDs:      []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90093000"}},
		Children: children,
	}}
	h.file("Hotel Show/Season 01/Hotel Show S01E01-thumb.jpg", 3)
	const cachedStill = "https://image.tmdb.org/t/p/original/hotel-s01e02.jpg"
	key := librarystore.LibraryCatalogKey{Provider: "tvdb", Kind: "episode", ExternalID: "90093002"}
	if _, err := h.st.SaveLibraryCatalog(ctx, key, `{"Source":{"Provider":"tmdb","Basis":"catalog","Catalog":"tmdb"},
		"Kind":"episode","Descriptive":{"Title":"Check Out"},"Artwork":[{"Kind":"thumb","Ref":"`+cachedStill+`"}]}`,
		catalogVersions[key.Kind], "", time.Now()); err != nil {
		t.Fatal(err)
	}
	h.im.writerVersion = stillsImported - 1
	h.run() // the earlier build: no still imported
	art := &fakeArtwork{}
	h.im.Artwork = art
	reads := h.reads.Load()

	h.run()
	if len(art.held[mp.KindThumb]) != 0 {
		t.Fatalf("an episode at the earlier version was written again by it: %v", art.held)
	}

	h.im.writerVersion = 0
	h.run()
	item := func(n int) string { return h.fileRow(filepath.Join(h.root, filepath.FromSlash(rel(n)))).ItemID }
	if got := art.held[mp.KindThumb][item(1)]; got != "Hotel Show S01E01-thumb.jpg" {
		t.Errorf("episode 1 still after the rewrite = %q, want the file beside it", got)
	}
	if got, want := art.refs[item(2)], "https://image.tmdb.org/t/p/w300/hotel-s01e02.jpg"; got != want {
		t.Errorf("episode 2 still after the rewrite = %q, want its cached record's at w300 %q", got, want)
	}
	if got := h.reads.Load(); got != reads {
		t.Errorf("headers read %d times by the rewrite, want none", got-reads)
	}
}

// A movie the walk writes while its catalog record is still to come is written again by the header
// stage once the record is cached; the poster that record names is imported then, with no later
// pass needed.
func TestImportPosterOfARecordThatArrivesBeforeTheHeaderRead(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Catalog = h.st
	art := &fakeArtwork{}
	h.im.Artwork = art
	h.file("Alpha (2001) {tmdb-90000901}/Alpha (2001) {tmdb-90000901}.mkv", 10)
	key := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000901"}
	h.im.Store = &savingFiles{ImportStore: h.im.Store, before: func() {
		if _, err := h.st.SaveLibraryCatalog(ctx, key, `{"Source":{"Provider":"tmdb","Basis":"catalog","Catalog":"tmdb"},"Kind":"movie","Descriptive":{"Title":"Alpha"},
			"Artwork":[{"Kind":"poster","Ref":"https://image.invalid/alpha.jpg"}]}`, catalogVersions[key.Kind], "", time.Now()); err != nil {
			t.Fatal(err)
		}
	}}
	h.run()
	if got := h.posters(art)["Alpha"]; got != "alpha.jpg" {
		t.Fatalf("poster = %q, want the record's alpha.jpg from the header stage's write", got)
	}
}

// An episode the walk writes while its catalog record is still to come is written again by the
// header stage once the record is cached (#4476), and the still the record names at TMDB's
// original size is handed over at w300.
func TestImportStillOfARecordThatArrivesBeforeTheHeaderRead(t *testing.T) {
	ctx := context.Background()
	sonarr := &fakeLister{}
	h := newHarness(t, Manager{Authority: "sonarr", Lister: sonarr})
	if err := h.st.RecordLibraryRootDiscovery(ctx, librarystore.LibraryRootDiscovery{
		Found: []librarystore.LibraryRootPath{{Authority: "sonarr", AuthorityPath: "/tv", RootPath: h.root}}, Complete: []string{"sonarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.im.Catalog = h.st
	art := &fakeArtwork{}
	h.im.Artwork = art
	const rel = "India Show/Season 01/India Show S01E01.mkv"
	h.file(rel, 10)
	sonarr.records = []mp.Record{{
		Source: mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager}, Kind: mp.KindSeries,
		IDs: []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90094000"}},
		Children: []mp.Record{{
			Source: mp.Source{Provider: mp.IDSonarr, Basis: mp.BasisManager}, Kind: mp.KindEpisode,
			IDs:       []mp.ExternalID{{Namespace: mp.NamespaceTVDB, Value: "90094001"}},
			Numbering: &mp.Numbering{Season: 1, Episode: 1},
			Files:     []mp.FileRef{{Path: "/tv/" + rel}},
		}},
	}}
	// The series' record is cached; the episode's arrives while the walk saves the episode.
	cache := func(key librarystore.LibraryCatalogKey, record string) {
		t.Helper()
		if _, err := h.st.SaveLibraryCatalog(ctx, key, record, catalogVersions[key.Kind], "", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	cache(librarystore.LibraryCatalogKey{Provider: "tvdb", Kind: "series", ExternalID: "90094000"},
		`{"Source":{"Provider":"tmdb","Basis":"catalog","Catalog":"tmdb"},"Kind":"series","Descriptive":{"Title":"India Show"}}`)
	h.im.Store = &savingFiles{ImportStore: h.im.Store, before: func() {
		cache(librarystore.LibraryCatalogKey{Provider: "tvdb", Kind: "episode", ExternalID: "90094001"},
			`{"Source":{"Provider":"tmdb","Basis":"catalog","Catalog":"tmdb"},"Kind":"episode","Descriptive":{"Title":"Arrival"},
			"Artwork":[{"Kind":"thumb","Ref":"https://image.tmdb.org/t/p/original/india-s01e01.jpg"}]}`)
	}}
	h.run()
	row := h.fileRow(filepath.Join(h.root, filepath.FromSlash(rel)))
	if strings.HasPrefix(row.Identity, "waiting:") {
		t.Fatalf("identity = %q: the header stage did not write the episode with its record", row.Identity)
	}
	if got, want := art.refs[row.ItemID], "https://image.tmdb.org/t/p/w300/india-s01e01.jpg"; got != want {
		t.Fatalf("still = %q, want the record's at w300 %q from the header stage's write", got, want)
	}
}

// A folder holding several videos says nothing of any one of them: its poster is no one's.
func TestImportFolderPosterNeedsASoleVideo(t *testing.T) {
	h := newHarness(t)
	posters := &fakeArtwork{}
	h.im.Art, h.im.Artwork = localart.New(), posters
	h.file("Mixed/Delta (2001).mkv", 10)
	h.file("Mixed/Echo (2002).mkv", 10)
	h.file("Mixed/poster.jpg", 3)
	h.run()
	for name, poster := range h.posters(posters) {
		if poster != "" {
			t.Errorf("%s took the shared folder's poster %q", name, poster)
		}
	}
}
