package scan

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/store"
)

// An arr's event names folders in the arr's own paths; each is mapped to its folder here and passed
// over on its own, and a folder outside every root is left to the hourly walk.
func TestScannerArrChangedPassesTheMappedFolder(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := h.st.RecordLibraryRootDiscovery(ctx, store.LibraryRootDiscovery{
		Found: []store.LibraryRootPath{{Authority: "radarr", AuthorityPath: "/movies", RootPath: h.root}}, Complete: []string{"radarr"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		h.file("India/India "+twoDigits(i)+".mkv", 5)
	}
	h.run()
	sc := NewScanner(h.im, time.Hour)
	sc.Debounce = 20 * time.Millisecond

	h.file("Juliet (2009)/Juliet (2009).mkv", 10)
	queued, err := sc.ArrChanged(ctx, "radarr", []string{"/movies/Juliet (2009)", "/elsewhere/Kilo (2010)"})
	if err != nil || queued != 1 {
		t.Fatalf("queued = %d, %v; want 1 (the folder outside every root is skipped)", queued, err)
	}
	if queued, _ := sc.ArrChanged(ctx, "sonarr", []string{"/movies/Juliet (2009)"}); queued != 0 {
		t.Fatalf("another arr's path map was used: queued %d", queued)
	}
	go sc.passFolders(ctx) // folder passes only: Run's first full pass would find the file anyway
	juliet := fileOrigin(h.root, "Juliet (2009)/Juliet (2009).mkv")
	waitFor(t, "the event's file", func() bool {
		_, ok, err := h.inv.Item(ctx, inventory.ItemRef{Origin: &juliet})
		return err == nil && ok
	})
}
