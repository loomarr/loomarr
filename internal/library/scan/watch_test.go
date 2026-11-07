package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
)

// A file that lands in a new folder of a local root is in the inventory within moments, with no
// walk of the whole root; a removed one is marked missing the same way.
func TestWatcherPassesOverChangedFolders(t *testing.T) {
	h := newHarness(t)
	if !localFilesystem(h.root) {
		t.Skip("the test folder is on a network filesystem")
	}
	for i := range 12 {
		h.file("Xray/Xray "+twoDigits(i)+".mkv", 5)
	}
	h.run()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &Watcher{Scanner: NewScanner(h.im, time.Hour), Debounce: 50 * time.Millisecond, Resync: time.Hour}
	go w.Run(ctx)
	time.Sleep(300 * time.Millisecond) // the watches are added

	h.file("Yankee (2007)/Yankee (2007).mkv", 10)
	added := fileOrigin(h.root, "Yankee (2007)/Yankee (2007).mkv")
	waitFor(t, "the new file's item", func() bool {
		_, ok, err := h.inv.Item(ctx, inventory.ItemRef{Origin: &added})
		return err == nil && ok
	})

	removeFile(t, h.root, "Xray/Xray 00.mkv")
	gone := fileOrigin(h.root, "Xray/Xray 00.mkv")
	waitFor(t, "the removed file marked missing", func() bool {
		item, ok, err := h.inv.Item(ctx, inventory.ItemRef{Origin: &gone})
		if err != nil || !ok {
			return false
		}
		for _, o := range item.Origins {
			if o.Key == gone {
				return !o.MissingAt.IsZero()
			}
		}
		return false
	})
	if roots, err := h.st.LibraryRoots(ctx); err != nil || len(roots.Roots) != 1 {
		t.Fatalf("roots = %+v, %v", roots, err)
	}
}

func removeFile(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
