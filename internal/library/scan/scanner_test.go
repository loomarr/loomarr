package scan

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A pass the watch wakes the scanner for, having lost track of what changed, is nobody's ask: it
// is quiet unless it finds work, as the hourly one is. Scan now is asked for.
func TestAPassTheWatchWakesForIsNotAskedFor(t *testing.T) {
	tracker := &Tracker{}
	sc := NewScanner(&Importer{Tracker: tracker}, time.Hour)
	pending := func() bool {
		tracker.mu.Lock()
		defer tracker.mu.Unlock()
		return tracker.pending
	}
	sc.PassSoon()
	if len(sc.trigger) != 1 || pending() {
		t.Fatalf("PassSoon: woken %v, asked %v; want woken, not asked", len(sc.trigger) == 1, pending())
	}
	<-sc.trigger
	sc.Trigger()
	if len(sc.trigger) != 1 || !pending() {
		t.Fatalf("Trigger: woken %v, asked %v; want both", len(sc.trigger) == 1, pending())
	}
}

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
// A folder asked for is unsettled from the ask until its pass has run, and the end of the round is
// told: that is what lets step 3 wait for its own rewrites and no longer.
func TestScannerSettlesOnceItsFolderPassesHaveRun(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.file("Lima (2011)/Lima (2011).mkv", 10)
	h.run()
	sc := NewScanner(h.im, time.Hour)
	sc.Debounce = 20 * time.Millisecond
	rounds := make(chan bool, 4)
	sc.AfterFolders = func() { rounds <- sc.FoldersSettled() }
	if !sc.FoldersSettled() {
		t.Fatal("a scanner asked for nothing is unsettled")
	}

	sc.PassFolders(h.root, filepath.Join(h.root, "Lima (2011)"))
	if sc.FoldersSettled() {
		t.Fatal("settled with a folder waiting for its pass")
	}
	go sc.passFolders(ctx)
	select {
	case settled := <-rounds:
		if !settled {
			t.Fatal("unsettled when the round's end was told")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the round's end was never told")
	}
}

// A folder pass's header reads wait while the household watches, but the next folders' passes do
// not wait behind them: a new episode an arr names is a title at once, and its file is read once
// the viewer stops.
func TestAFolderPassIsNotHeldBehindReadsWaitingForViewers(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.run()
	var busy atomic.Bool
	busy.Store(true)
	h.im.Busy = busy.Load
	h.im.Poll = 10 * time.Millisecond
	sc := NewScanner(h.im, time.Hour)
	sc.Debounce = 10 * time.Millisecond
	rounds := make(chan struct{}, 4)
	sc.AfterFolders = func() { rounds <- struct{}{} }
	done := make(chan struct{})
	go func() { sc.passFolders(ctx); close(done) }()
	round := func(rel string) {
		p := h.file(rel, 10)
		sc.PassFolders(h.root, filepath.Dir(filepath.Dir(p)))
		select {
		case <-rounds:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: the folder pass waited for the viewer", rel)
		}
		if _, ok := ProjectLibraryItem(h.item(fileOrigin(h.root, rel))); !ok {
			t.Fatalf("%s is no title while the viewer watches", rel)
		}
	}
	round("Quillmoor/Season 1/Quillmoor - S01E01.mkv")
	round("Fallow Mere/Season 1/Fallow Mere - S01E01.mkv")
	if h.reads.Load() != 0 {
		t.Fatalf("%d headers read while the viewer watches", h.reads.Load())
	}
	busy.Store(false)
	deadline := time.Now().Add(5 * time.Second)
	for h.reads.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := h.reads.Load(); n < 2 {
		t.Fatalf("%d headers read once the viewer stopped; want both", n)
	}
	cancel()
	<-done
}

// A header stage tells of each folder once its files are read and saved, while it reads on, so a
// rewrite held for the folder need not wait for the whole stage.
func TestAHeaderStageTellsOfEachFolderItHasRead(t *testing.T) {
	h := newHarness(t)
	const films = headerBatch + 10
	for i := range films {
		h.file(fmt.Sprintf("Film %02d (2001)/Film %02d.mkv", i, i), 10)
	}
	var mu sync.Mutex
	told := map[string]bool{}
	earliest := int32(-1)
	ended := false
	h.im.FolderRead = func(root, dir string) {
		mu.Lock()
		defer mu.Unlock()
		if ended {
			t.Errorf("%s told after the stage ended", dir)
		}
		if earliest < 0 {
			earliest = h.reads.Load()
		}
		rows, err := h.st.LibraryFiles(context.Background(), root)
		if err != nil {
			t.Error(err)
		}
		for _, r := range rows {
			if filepath.Dir(r.Path) == dir && r.Header != librarystore.LibraryHeaderRead {
				t.Errorf("%s told before %s was saved read", dir, r.Path)
			}
		}
		told[dir] = true
	}
	h.im.AfterReading = func() {
		mu.Lock()
		ended = true
		mu.Unlock()
	}
	h.run()
	if earliest < 0 || earliest >= films {
		t.Fatalf("the first folder was told after %d of %d reads; want while the stage read on", earliest, films)
	}
	if len(told) < headerBatch {
		t.Fatalf("%d folders told while the stage read, want at least its first batch's %d", len(told), headerBatch)
	}
}

// The Enricher's rewrite of a folder waits while a header stage has its files to read: a file still
// to be read takes the details when its read writes it, and a pass meanwhile would make its read
// stale. The held folder keeps step 3 unsettled, and goes to its pass once its files are read and
// saved, or the stage ends. A folder the stage holds nothing of is passed over at once.
func TestScannerHoldsRewritesWhileFilesAreRead(t *testing.T) {
	h := newHarness(t)
	sc := NewScanner(h.im, time.Hour)
	lima := filepath.Join(h.root, "Lima (2011)")
	mike := filepath.Join(h.root, "Mike (2012)")
	// A header stage tells of its end, once it has read and saved its files.
	ended := 0
	h.im.AfterReading = func() { ended++ }
	h.file("Lima (2011)/Lima (2011).mkv", 10)
	h.run()
	if ended != 1 || h.im.Reading() {
		t.Fatalf("a header stage's end told %d times, still reading %v", ended, h.im.Reading())
	}

	h.im.mu.Lock()
	h.im.reading, h.im.unread = true, map[string]int{lima: 1}
	h.im.mu.Unlock()
	sc.Rewrite(h.root, lima, mike)
	if _, ok := sc.folders[h.root][lima]; ok {
		t.Fatalf("a rewrite passed over a folder whose files were being read: %v", sc.folders)
	}
	if _, ok := sc.folders[h.root][mike]; !ok {
		t.Fatalf("a rewrite of a folder with nothing to read was held: %v", sc.folders)
	}
	delete(sc.folders, h.root)
	if sc.FoldersSettled() {
		t.Fatal("settled with a rewrite held")
	}

	// Its files read and saved, the folder goes to its pass while the stage reads on.
	sc.ReleaseFolder(h.root, lima)
	if _, ok := sc.folders[h.root][lima]; !ok || len(sc.held) != 0 {
		t.Fatalf("the held rewrite did not go to its pass once its folder was read: folders %v, held %v", sc.folders, sc.held)
	}
	delete(sc.folders, h.root)
	sc.Rewrite(h.root, lima)
	delete(sc.folders, h.root)

	h.im.mu.Lock()
	h.im.reading = false
	h.im.mu.Unlock()
	sc.ReleaseHeld()
	if _, ok := sc.folders[h.root][lima]; !ok || len(sc.held) != 0 {
		t.Fatalf("the held rewrite did not go to its pass: folders %v, held %v", sc.folders, sc.held)
	}

	// With nothing read, a rewrite goes straight to its pass.
	delete(sc.folders, h.root)
	sc.Rewrite(h.root, lima)
	if _, ok := sc.folders[h.root][lima]; !ok {
		t.Fatalf("a rewrite with nothing read was held: %v", sc.folders)
	}
}

// A pass asked for while a first import reads its headers walks at once: a folder added, or Scan
// now, finds its new files then, not once every header is read. Its files go to the stage reading.
func TestAScanAskedForWhileFilesAreReadWalksAtOnce(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	h.file("Lima (2011)/Lima (2011).mkv", 10)
	read := h.im.Header
	release := make(chan struct{})
	reading := make(chan struct{}, 8)
	h.im.Header = func(ctx context.Context, path string) (header.Header, error) {
		reading <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return read(ctx, path)
	}
	sc := NewScanner(h.im, time.Hour)
	done := make(chan struct{})
	go func() { defer close(done); sc.Run(ctx) }()
	defer func() {
		cancel()
		<-done
	}()
	select {
	case <-reading:
	case <-time.After(10 * time.Second):
		t.Fatal("the first import never read a header")
	}

	juliet := h.file("Juliet (2009)/Juliet (2009).mkv", 10)
	sc.Trigger()
	waitFor(t, "the new file walked while the first reads wait", func() bool {
		rows, err := h.st.LibraryFiles(ctx, h.root)
		return err == nil && slices.ContainsFunc(rows, func(r librarystore.LibraryFile) bool { return r.Path == juliet })
	})
	close(release)
	waitFor(t, "both files read", func() bool {
		rows, err := h.st.LibraryFiles(ctx, h.root)
		return err == nil && len(rows) == 2 && !slices.ContainsFunc(rows, func(r librarystore.LibraryFile) bool {
			return r.Header != librarystore.LibraryHeaderRead
		})
	})
}

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
