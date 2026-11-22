package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The real disk says a regular file is there, and nothing else is: a folder, or a path with
// nothing at it.
func TestOSDiskFindsOnlyARegularFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "film.mkv")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for path, want := range map[string]bool{file: true, dir: false, filepath.Join(dir, "gone.mkv"): false} {
		if got := (OSDisk{}).File(ctx, path); got != want {
			t.Errorf("File(%s) = %v, want %v", path, got, want)
		}
	}
}
