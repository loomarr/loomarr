package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// A file renamed or moved keeps the header read at its old path and is not opened again; the old
// path is gone. Two vanished files with the same size and time say nothing, and each new one is read.
func TestImportMovedFileKeepsItsHeader(t *testing.T) {
	h := newHarness(t)
	for i := range 12 {
		h.file(fmt.Sprintf("Filler %02d/Filler %02d.mkv", i, i), 5+i)
	}
	old := h.file("Lima/Lima 720p.mkv", 100)
	h.run()
	read := h.reads.Load()

	moved := filepath.Join(h.root, "Lima (2004)", "Lima (2004).mkv")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(old, moved); err != nil {
		t.Fatal(err)
	}
	h.run()
	if got := h.reads.Load(); got != read {
		t.Fatalf("header reads = %d, want %d: the moved file was opened again", got, read)
	}
	origin := fileOrigin(h.root, "Lima (2004)/Lima (2004).mkv")
	if header := h.im.lastHeader(t.Context(), origin); header == nil || len(header.Streams) == 0 {
		t.Fatalf("moved file's header = %+v, want the one read before", header)
	}
	rows, err := h.st.LibraryFiles(t.Context(), h.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		switch r.Path {
		case moved:
			if r.Header != store.LibraryHeaderRead {
				t.Errorf("moved row header = %q", r.Header)
			}
		case old:
			if r.MissingAt.IsZero() {
				t.Error("the old path is still present")
			}
		}
	}

	// Twins: same size, same time. Neither vanished one names a new file.
	a, b := h.file("Mike/a.mkv", 50), h.file("Mike/b.mkv", 50)
	same := mustStat(t, a).ModTime()
	if err := os.Chtimes(b, same, same); err != nil {
		t.Fatal(err)
	}
	h.run()
	read = h.reads.Load()
	for _, p := range []string{a, b} {
		if err := os.Rename(p, p+".moved.mkv"); err != nil {
			t.Fatal(err)
		}
	}
	h.run()
	if got := h.reads.Load(); got != read+2 {
		t.Fatalf("header reads = %d, want %d: twins were matched by guesswork", got, read+2)
	}
}

func mustStat(t *testing.T, p string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
