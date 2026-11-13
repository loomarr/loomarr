package playoutresolve

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// fakeLibraryFiles plays one whole file per item.
type fakeLibraryFiles map[string]string

func (f fakeLibraryFiles) LibraryPlayingParts(_ context.Context, itemID string) ([]librarystore.LibraryPlayingPart, error) {
	path, ok := f[itemID]
	if !ok {
		return nil, nil
	}
	return []librarystore.LibraryPlayingPart{{Path: path}}, nil
}

// fakeLibraryParts plays each item's parts in order.
type fakeLibraryParts map[string][]librarystore.LibraryPlayingPart

func (f fakeLibraryParts) LibraryPlayingParts(_ context.Context, itemID string) ([]librarystore.LibraryPlayingPart, error) {
	return f[itemID], nil
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
	r := &Resolver{libraryFile: fakeLibraryFiles{"lib-1": film, "lib-gone": filepath.Join(dir, "gone.mkv")}}
	ctx := context.Background()
	if got := r.itemInput(ctx, "lib-1"); got != film {
		t.Errorf("library item = %q, want its file", got)
	}
	for _, id := range []string{"lib-gone", "90000001"} {
		if got := r.itemInput(ctx, id); got != "" {
			t.Errorf("%s = %q, want nothing to play", id, got)
		}
	}
	if got := (&Resolver{}).itemInput(ctx, "lib-1"); got != "" {
		t.Errorf("with no Library = %q, want nothing to play", got)
	}
	if readableFile(dir) {
		t.Error("a directory is not a readable file")
	}
}
