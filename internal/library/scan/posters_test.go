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

// fakePosters keeps each title's poster file name; fail makes every import fail and change nothing.
type fakePosters struct {
	fail bool
	held map[string]string
}

func (f *fakePosters) Poster(_ context.Context, itemID, ref string) error {
	if f.fail {
		return errors.New("worker down")
	}
	if f.held == nil {
		f.held = map[string]string{}
	}
	if ref == "" {
		delete(f.held, itemID)
		return nil
	}
	f.held[itemID] = filepath.Base(ref)
	return nil
}

// posters is each listed title's poster, by name.
func (h *harness) posters(f *fakePosters) map[string]string {
	h.t.Helper()
	page, err := h.st.ListLibraryItems(context.Background(), store.LibraryItemQuery{})
	if err != nil {
		h.t.Fatal(err)
	}
	out := map[string]string{}
	for _, it := range page.Items {
		out[it.Name] = f.held[it.ID]
	}
	return out
}

// The scanner imports each title's poster from beside its media: a movie folder's poster, then the
// movie's own when one appears, and a series folder's. A poster deleted is let go. An import that
// fails changes nothing and the pass carries on.
func TestImportPostersFromBesideTheMedia(t *testing.T) {
	h := newHarness(t)
	posters := &fakePosters{}
	h.im.Art, h.im.Posters = localart.New(), posters
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
	posters := &fakePosters{}
	h.im.Art, h.im.Posters = localart.New(), posters
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
