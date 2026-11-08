package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/metadataprovider/localart"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// fakeArtwork keeps each title's image file name per kind; fail makes every import fail and
// change nothing.
type fakeArtwork struct {
	fail bool
	held map[string]map[string]string // kind → item → file name
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
	return nil
}

// art is each listed title's image of one kind, by name.
func (h *harness) art(f *fakeArtwork, kind string) map[string]string {
	h.t.Helper()
	page, err := h.st.ListLibraryItems(context.Background(), store.LibraryItemQuery{})
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
	return h.art(f, localart.KindPoster)
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
		localart.KindPoster: "poster.jpg", localart.KindFanart: "fanart.jpg", localart.KindClearlogo: "clearlogo.png",
	} {
		if got := h.art(art, kind)["Alpha"]; got != want {
			t.Errorf("%s = %q, want %q", kind, got, want)
		}
	}
	if len(art.held[localart.KindBanner]) != 0 {
		t.Errorf("banner imported: %v", art.held[localart.KindBanner])
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
