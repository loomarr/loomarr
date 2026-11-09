package scan

import (
	"context"
	"io/fs"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// A header an older reader left is read once more by this one. Until then the file keeps its
// facts and its title stays ready, and a header stage the share cuts short is taken up again on
// the next pass, for the files it did not reach.
func TestAnOlderReadersHeaderIsReadAgain(t *testing.T) {
	h := newHarness(t)
	h.file("Kilo (2001) {tmdb-90000651}/Kilo (2001).mkv", 10)
	h.file("Lima (2004) {tmdb-90000652}/Lima (2004).mkv", 10)
	h.im.readerVersion = 1
	h.run()
	if got := h.reads.Load(); got != 2 {
		t.Fatalf("first pass read %d headers, want 2", got)
	}

	// This reader; the share stops answering after one header.
	h.im.readerVersion = 0
	read := h.im.Header
	// The header stage reads several files at once: the check and the read are one step, or two
	// workers both see "one read so far" and both read.
	var gate sync.Mutex
	h.im.Header = func(ctx context.Context, path string) (header.Header, error) {
		gate.Lock()
		defer gate.Unlock()
		if h.reads.Load() >= 3 {
			h.reads.Add(1)
			return header.Header{}, fs.ErrNotExist
		}
		return read(ctx, path)
	}
	h.run()
	if got := h.reads.Load(); got != 4 {
		t.Fatalf("second pass tried %d headers in all, want 4: one read again, then the share stopped", got)
	}
	versions := map[int]int{}
	for _, rel := range []string{"Kilo (2001) {tmdb-90000651}/Kilo (2001).mkv", "Lima (2004) {tmdb-90000652}/Lima (2004).mkv"} {
		facts, by := h.im.lastHeader(t.Context(), fileOrigin(h.root, rel))
		if facts == nil || len(facts.Streams) == 0 {
			t.Fatalf("%s lost its header facts while waiting to be read again", rel)
		}
		versions[by]++
	}
	if versions[1] != 1 || versions[header.Version] != 1 {
		t.Fatalf("readers = %v, want one file read again and one still waiting", versions)
	}
	page, err := h.st.ListLibraryItems(t.Context(), store.LibraryItemQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range page.Items {
		if it.Pending != 0 {
			t.Errorf("%s is getting ready again; an older reader's facts should stand", it.Name)
		}
	}

	// The share is back: the file left behind is read, and then nothing is.
	h.im.Header = read
	h.run()
	if got := h.reads.Load(); got != 5 {
		t.Fatalf("third pass: %d headers in all, want the one left behind read (5)", got)
	}
	h.run()
	if got := h.reads.Load(); got != 5 {
		t.Fatalf("fourth pass: %d headers in all, want none read again (5)", got)
	}
}
