package scan

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// A video's trickplay file beside it, by its name, is referenced with its source; another video's
// in the same folder is not, and a file that gains one is written again.
func TestAFileReferencesItsTrickplay(t *testing.T) {
	h := newHarness(t)
	season := "Ember Coast/Season 01/"
	one := h.file(season+"Ember Coast S01E01.mkv", 10)
	h.file(season+"Ember Coast S01E02.mkv", 10)
	h.run()
	write := func(name string) string {
		t.Helper()
		path := filepath.Join(h.root, filepath.FromSlash(season), name)
		if err := os.WriteFile(path, []byte("\x89BIF"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	bif := write("Ember Coast S01E01.bif")
	h.run()

	trickplay := func(rel string) []store.LibraryTrickplay {
		t.Helper()
		sources := librarySources(h.item(fileOrigin(h.root, season+rel)))
		if len(sources) != 1 {
			t.Fatalf("%s sources = %+v", rel, sources)
		}
		return sources[0].Trickplay
	}
	if got := trickplay("Ember Coast S01E01.mkv"); !slices.Equal(got, []store.LibraryTrickplay{{Ref: bif, Format: "bif"}}) {
		t.Errorf("%s trickplay = %+v, want its own .bif", one, got)
	}
	if got := trickplay("Ember Coast S01E02.mkv"); len(got) != 0 {
		t.Errorf("episode 2 took episode 1's trickplay: %+v", got)
	}
}
