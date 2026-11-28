package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A show waiting in Needs you lists each file's length and asks for a frame of its main file. A
// pass that renames the main file drops the frame and asks for one of the new file, and once the
// show is answered it is asked for none and loses the one it had.
func TestNeedFrameFollowsItsMainFile(t *testing.T) {
	const ep1 = "Garden Diary/Season 01/Garden Diary S01E01.mkv"
	ctx := context.Background()
	h := newHarness(t)
	h.im.Answers, h.im.Catalog = h.st, h.st
	// Enough episodes that a rename is not held for the maintainer to confirm.
	for n := 1; n <= 11; n++ {
		h.file(fmt.Sprintf("Garden Diary/Season 01/Garden Diary S01E%02d.mkv", n), 10)
	}
	h.run()
	lib := &Library{Roots: &Roots{Store: h.st}, NeedStore: h.st, ItemStore: h.st, Inventory: h.inv, Keys: fakeCatalogKeys}
	needs, err := lib.Needs(ctx)
	if err != nil || len(needs) != 1 || len(needs[0].Present) != 11 {
		t.Fatalf("needs = %+v, %v; want the show with its 11 files", needs, err)
	}
	for _, f := range needs[0].Present {
		if f.DurationMs <= 0 {
			t.Fatalf("%s has no length", f.Path)
		}
	}
	id := needs[0].ID
	wanted := func() []librarystore.LibraryFrameFile {
		t.Helper()
		frames, err := h.st.LibraryNeedsWithoutFrame(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return frames
	}
	before := wanted()
	if len(before) != 1 || before[0].ItemID != id || before[0].Path != filepath.Join(h.root, filepath.FromSlash(ep1)) {
		t.Fatalf("wanted = %+v, want a frame of %s", before, ep1)
	}
	at := h.im.now()
	img := clipstore.Image{Hash: strings.Repeat("e", 64), Origin: "extracted", Visibility: "member",
		Role: "thumb", MIME: "image/jpeg", Width: 16, Height: 9, Bytes: 1, FrameCount: 1,
		CreatedAt: at, UpdatedAt: at, LastUsedAt: at}
	if err := h.st.PutImage(ctx, img); err != nil {
		t.Fatal(err)
	}
	if put, err := h.st.PutLibraryNeedFrame(ctx, before[0], img.Hash); err != nil || !put {
		t.Fatalf("put = %v, %v", put, err)
	}
	h.run()
	if got := wanted(); len(got) != 0 {
		t.Fatalf("a pass that changed nothing dropped the frame: %+v", got)
	}

	from := filepath.Join(h.root, filepath.FromSlash(ep1))
	renamed := strings.TrimSuffix(from, ".mkv") + " 1080p.mkv"
	if err := os.Rename(from, renamed); err != nil {
		t.Fatal(err)
	}
	h.run()
	if got := wanted(); len(got) != 1 || got[0].Path != renamed {
		t.Fatalf("wanted = %+v, want a new frame of %s", got, renamed)
	}
	if put, err := h.st.PutLibraryNeedFrame(ctx, wanted()[0], img.Hash); err != nil || !put {
		t.Fatalf("put again = %v, %v", put, err)
	}

	if err := lib.Ignore(ctx, id); err != nil {
		t.Fatal(err)
	}
	h.run()
	if got := wanted(); len(got) != 0 {
		t.Fatalf("an answered show is still asked for a frame: %+v", got)
	}
	owned, err := h.st.ImagesForOwner(ctx, librarystore.LibraryItemImageOwner, id)
	if err != nil || len(owned) != 0 {
		t.Fatalf("an answered show kept %d images, %v", len(owned), err)
	}
}
