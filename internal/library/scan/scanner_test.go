package scan

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Each folder waits on its own: a quiet one is due while a busy one keeps waiting, and the busy one
// is due anyway once it has waited MaxWait.
func TestDueFoldersWaitEachOnTheirOwn(t *testing.T) {
	sc := NewScanner(&Importer{}, time.Hour)
	sc.Debounce, sc.MaxWait = 5*time.Second, time.Minute
	t0 := time.UnixMilli(1_790_000_000_000)
	sc.folders = map[string]map[string]folderWait{
		"/media": {
			"/media/Quiet": {first: t0, last: t0},
			// Changing every second, the latest just now.
			"/media/Busy": {first: t0, last: t0.Add(10 * time.Second)},
		},
	}
	due, next := sc.dueFolders(t0.Add(10 * time.Second))
	if len(due["/media"]) != 1 || due["/media"][0] != "/media/Quiet" || next != 5*time.Second {
		t.Fatalf("due = %v, next %v; want the quiet folder, the busy one in 5s", due, next)
	}
	sc.folders["/media"]["/media/Busy"] = folderWait{first: t0, last: t0.Add(59 * time.Second)}
	due, next = sc.dueFolders(t0.Add(time.Minute))
	if len(due["/media"]) != 1 || due["/media"][0] != "/media/Busy" || next != 0 || len(sc.folders) != 0 {
		t.Fatalf("due = %v, next %v, left %v; want the busy folder at its max wait", due, next, sc.folders)
	}
}

// An arr's event names folders in the arr's own paths; each is mapped to its folder here and passed
// over on its own, and a folder outside every root is left to the hourly walk.
func TestScannerArrChangedPassesTheMappedFolder(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := h.st.RecordLibraryRootDiscovery(ctx, librarystore.LibraryRootDiscovery{
		Found: []librarystore.LibraryRootPath{{Authority: "radarr", AuthorityPath: "/movies", RootPath: h.root}}, Complete: []string{"radarr"},
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
