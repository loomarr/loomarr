package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
)

type fakeLibraryFiles map[string]string

func (f fakeLibraryFiles) LibraryPlayingFile(_ context.Context, itemID string) (string, bool, error) {
	path, ok := f[itemID]
	return path, ok, nil
}

// A programme reads its library item's own file, with no media server. A file that is gone, or an
// id the Library does not hold (a media server's, say), has nothing to play: playout never streams
// from the media server (#2151 step 16).
func TestPlayoutItemInputReadsOnlyALibraryItemsOwnFile(t *testing.T) {
	dir := t.TempDir()
	film := filepath.Join(dir, "film.mkv")
	if err := os.WriteFile(film, []byte("matroska"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &playoutResolver{libraryFile: fakeLibraryFiles{"lib-1": film, "lib-gone": filepath.Join(dir, "gone.mkv")}}
	ctx := context.Background()
	if got := r.itemInput(ctx, "lib-1"); got.URL != film || got.Kind != library.InputFile {
		t.Errorf("library item = %+v, want its file", got)
	}
	for _, id := range []string{"lib-gone", "90000001"} {
		if got := r.itemInput(ctx, id); got.URL != "" {
			t.Errorf("%s = %+v, want nothing to play", id, got)
		}
	}
	if got := (&playoutResolver{}).itemInput(ctx, "lib-1"); got.URL != "" {
		t.Errorf("with no Library = %+v, want nothing to play", got)
	}
}
