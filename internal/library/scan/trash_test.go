package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Two files of one movie: the sharper plays and the other is an extra copy. Removing the copy that
// plays is refused; removing the extra moves it into the folder's trash, out of the library, where
// walks don't find it, until a walk 30 days on empties that day.
func TestRemoveExtraCopyMovesItToTheTrash(t *testing.T) {
	h := newHarness(t)
	h.im.Header = func(_ context.Context, path string) (header.Header, error) {
		w, ht := 1920, 1080
		if strings.Contains(path, "DVD") {
			w, ht = 720, 480
		}
		return header.Header{Container: "matroska,webm", DurationMillis: 5_000_000, Streams: []inventory.Stream{
			{Index: 0, Kind: inventory.StreamVideo, Codec: "h264", Width: w, Height: ht},
		}}, nil
	}
	sharp := h.file("Foxtrot (2001) {tmdb-90000631}/Foxtrot.mkv", 10)
	soft := h.file("Foxtrot (2001) {tmdb-90000631}/Foxtrot DVD.mkv", 20)
	h.run()
	id := string(h.item(fileOrigin(h.root, "Foxtrot (2001) {tmdb-90000631}/Foxtrot.mkv")).ID)

	day := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	lib := &Library{ItemStore: h.st, Inventory: h.inv, Now: func() time.Time { return day }}
	ctx := context.Background()
	d, err := lib.Item(ctx, id, false)
	if err != nil || len(d.Files) != 2 {
		t.Fatalf("title = %+v, %v; want one title with both files", d.Files, err)
	}
	for _, f := range d.Files {
		if (f.State == librarystore.LibraryCopyExtra) != (f.Path == soft) {
			t.Errorf("%s state = %s", f.Path, f.State)
		}
	}

	if _, err := lib.RemoveExtraCopy(ctx, id, sharp); !errors.Is(err, ErrPlays) {
		t.Fatalf("removing the copy that plays = %v, want ErrPlays", err)
	}
	if _, err := lib.RemoveExtraCopy(ctx, id, "/elsewhere/Foxtrot.mkv"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("removing another title's file = %v, want ErrNotFound", err)
	}
	if _, err := lib.RemoveExtraCopy(ctx, id, soft); err != nil {
		t.Fatal(err)
	}
	trashed := filepath.Join(h.root, TrashDir, "2026-11-07", "Foxtrot (2001) {tmdb-90000631}", "Foxtrot DVD.mkv")
	if _, err := os.Stat(trashed); err != nil {
		t.Fatalf("trashed copy: %v", err)
	}
	if _, err := os.Stat(soft); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the extra copy is still in place: %v", err)
	}

	// Half the movie's files went, which a walk would hold as a likely unplugged drive; Loomarr
	// moved this one itself, so nothing is held.
	if d, err = lib.Item(ctx, id, false); err != nil || len(d.Files) != 1 || d.Files[0].Path != sharp || d.Files[0].State != librarystore.LibraryCopyPlays {
		t.Fatalf("title after removal = %+v, %v; want the copy that plays alone", d.Files, err)
	}
	h.run()
	if c, err := h.st.LibraryFileCounts(ctx); err != nil || len(c.Held) != 0 || c.Files != 1 {
		t.Fatalf("counts after the next walk = %+v, %v; want one file and nothing held", c, err)
	}

	// A folder Loomarr didn't make is left alone; the day's folder goes 30 days on.
	keep := filepath.Join(h.root, TrashDir, "mine")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := emptyTrash(h.root, day.Add(29*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(trashed); err != nil {
		t.Fatalf("emptied before 30 days: %v", err)
	}
	if err := emptyTrash(h.root, day.Add(TrashKeep)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(filepath.Dir(trashed))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the day's folder is still there: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("emptying removed a folder Loomarr didn't make: %v", err)
	}
}

// A copy that plays can go from the file list before its source row does (a pass that could not
// hear from an arr marks no origin unseen). The extra copy left alone is then the title's last
// file: it is not an extra, and removing it is refused.
func TestATitlesLastFileIsNeverAnExtraCopy(t *testing.T) {
	h := newHarness(t)
	h.im.Header = func(_ context.Context, path string) (header.Header, error) {
		w, ht := 1920, 1080
		if strings.Contains(path, "DVD") {
			w, ht = 720, 480
		}
		return header.Header{Container: "matroska,webm", DurationMillis: 5_000_000, Streams: []inventory.Stream{
			{Index: 0, Kind: inventory.StreamVideo, Codec: "h264", Width: w, Height: ht},
		}}, nil
	}
	sharp := h.file("Foxtrot (2001) {tmdb-90000631}/Foxtrot.mkv", 10)
	soft := h.file("Foxtrot (2001) {tmdb-90000631}/Foxtrot DVD.mkv", 20)
	h.run()
	id := string(h.item(fileOrigin(h.root, "Foxtrot (2001) {tmdb-90000631}/Foxtrot.mkv")).ID)
	ctx := context.Background()
	// Only the file list changes: the sources still say the sharp copy plays.
	if err := h.st.RemoveLibraryFile(ctx, sharp, time.Now()); err != nil {
		t.Fatal(err)
	}

	lib := &Library{ItemStore: h.st, Inventory: h.inv}
	d, err := lib.Item(ctx, id, false)
	if err != nil || len(d.Files) != 1 || d.Files[0].Path != soft {
		t.Fatalf("title = %+v, %v; want only the soft copy listed", d.Files, err)
	}
	if d.Files[0].State == librarystore.LibraryCopyExtra {
		t.Error("the title's last file is an extra copy")
	}
	if _, err := lib.RemoveExtraCopy(ctx, id, soft); !errors.Is(err, ErrPlays) {
		t.Fatalf("removing the title's last file = %v, want ErrPlays", err)
	}
	if _, err := os.Stat(soft); err != nil {
		t.Errorf("the title's last file moved: %v", err)
	}
}

// The list can keep a copy that plays after it has gone from disk (a held removal, an arr not
// answering). Its extra copy still shows as one, but removing it is refused, and it stays.
func TestRemoveExtraCopyNeedsTheCopyThatPlaysOnDisk(t *testing.T) {
	h := newHarness(t)
	h.im.Header = func(_ context.Context, path string) (header.Header, error) {
		w, ht := 1920, 1080
		if strings.Contains(path, "DVD") {
			w, ht = 720, 480
		}
		return header.Header{Container: "matroska,webm", DurationMillis: 5_000_000, Streams: []inventory.Stream{
			{Index: 0, Kind: inventory.StreamVideo, Codec: "h264", Width: w, Height: ht},
		}}, nil
	}
	h.file("Foxtrot (2001) {tmdb-90000631}/Foxtrot.mkv", 10)
	soft := h.file("Foxtrot (2001) {tmdb-90000631}/Foxtrot DVD.mkv", 20)
	h.run()
	id := string(h.item(fileOrigin(h.root, "Foxtrot (2001) {tmdb-90000631}/Foxtrot.mkv")).ID)
	ctx := context.Background()

	// The disk holds only the soft copy; the list and the sources still hold both.
	disk := fakeDisk{files: map[string]bool{soft: true}}
	lib := &Library{Roots: &Roots{Disk: disk}, ItemStore: h.st, Inventory: h.inv}
	d, err := lib.Item(ctx, id, false)
	if err != nil || len(d.Files) != 2 {
		t.Fatalf("title = %+v, %v; want both files listed", d.Files, err)
	}
	for _, f := range d.Files {
		if (f.State == librarystore.LibraryCopyExtra) != (f.Path == soft) {
			t.Errorf("%s state = %s, want only the soft copy shown as an extra", f.Path, f.State)
		}
	}
	if _, err := lib.RemoveExtraCopy(ctx, id, soft); !errors.Is(err, ErrPlayingCopyUnseen) {
		t.Fatalf("removing the copy whose twin is gone from disk = %v, want ErrPlayingCopyUnseen", err)
	}
	if _, err := os.Stat(soft); err != nil {
		t.Errorf("the title's last real file moved: %v", err)
	}
}

// gateDisk answers File as its fakeDisk does, holding the first call until release is closed.
type gateDisk struct {
	fakeDisk
	once             *sync.Once
	entered, release chan struct{}
}

func (d gateDisk) File(ctx context.Context, path string) bool {
	d.once.Do(func() {
		close(d.entered)
		<-d.release
	})
	return d.fakeDisk.File(ctx, path)
}

// Two removals at once cannot trash both copies of a title. The first is held inside its check
// while a rescan flips which copy plays, and the second asks for the other copy meanwhile: it
// waits for the first, then sees its copy as the title's last and is refused. One file remains.
func TestTwoRemovalsAtOnceKeepOneCopy(t *testing.T) {
	h := newHarness(t)
	h.im.Header = func(_ context.Context, path string) (header.Header, error) {
		w, ht := 1920, 1080
		if strings.Contains(path, "DVD") {
			w, ht = 720, 480
		}
		return header.Header{Container: "matroska,webm", DurationMillis: 5_000_000, Streams: []inventory.Stream{
			{Index: 0, Kind: inventory.StreamVideo, Codec: "h264", Width: w, Height: ht},
		}}, nil
	}
	sharp := h.file("Foxtrot (2001) {tmdb-90000631}/Foxtrot.mkv", 10)
	soft := h.file("Foxtrot (2001) {tmdb-90000631}/Foxtrot DVD.mkv", 20)
	h.run()
	item := h.item(fileOrigin(h.root, "Foxtrot (2001) {tmdb-90000631}/Foxtrot.mkv"))
	id := string(item.ID)
	ctx := context.Background()

	disk := gateDisk{fakeDisk: fakeDisk{files: map[string]bool{sharp: true, soft: true}}, once: &sync.Once{},
		entered: make(chan struct{}), release: make(chan struct{})}
	lib := &Library{Roots: &Roots{Disk: disk}, ItemStore: h.st, Inventory: h.inv}
	errs := make(chan error, 2)
	remove := func(path string) {
		_, err := lib.RemoveExtraCopy(ctx, id, path)
		errs <- err
	}
	go remove(soft)
	<-disk.entered
	// A rescan meanwhile: the soft copy plays now, and the sharp one is the extra.
	row, ok := ProjectLibraryItem(item)
	if !ok {
		t.Fatal("the title is not listed")
	}
	for i := range row.Sources {
		row.Sources[i].Plays = !row.Sources[i].Plays
	}
	if err := h.st.SaveLibraryItems(ctx, []librarystore.LibraryItem{row}, time.Now()); err != nil {
		t.Fatal(err)
	}
	go remove(sharp)
	close(disk.release)

	var removed, refused int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			removed++
		case errors.Is(err, ErrPlays):
			refused++
		default:
			t.Fatalf("removal = %v", err)
		}
	}
	if removed != 1 || refused != 1 {
		t.Errorf("removed %d and refused %d, want one of each", removed, refused)
	}
	left := 0
	for _, p := range []string{sharp, soft} {
		if _, err := os.Stat(p); err == nil {
			left++
		}
	}
	if left != 1 {
		t.Errorf("%d copies left on disk, want 1", left)
	}
}

// The file an arr lists plays, however sharp another copy is; without one, the sharper, then the
// bigger. A copy under half as long as the longest never plays: a sharp trailer the walk took for a
// copy loses to the film. One whose length isn't read yet is not held back.
func TestTheCopyThatPlays(t *testing.T) {
	type copy struct {
		managed      bool
		pixels       int
		size, length int64
	}
	const hd, sd = 1920 * 1080, 720 * 480
	const film, trailer = 6_000_000, 150_000
	plays := func(copies []copy) []bool {
		return playingCopies(len(copies), func(int) string { return "" }, func(int) int { return 0 },
			func(i int) bool { return copies[i].managed }, func(i int) int { return copies[i].pixels },
			func(i int) int64 { return copies[i].size }, func(i int) int64 { return copies[i].length })
	}
	for name, tc := range map[string]struct {
		copies []copy
		plays  int
	}{
		"arr's file":                        {[]copy{{pixels: hd}, {pixels: sd, managed: true}}, 1},
		"sharper":                           {[]copy{{pixels: sd}, {pixels: hd}}, 1},
		"bigger":                            {[]copy{{size: 9}, {size: 3}}, 0},
		"a sharp trailer loses to the film": {[]copy{{pixels: hd, length: trailer}, {pixels: sd, length: film}}, 1},
		"even its arr's":                    {[]copy{{pixels: hd, length: trailer, managed: true}, {pixels: sd, length: film}}, 1},
		"a length not read yet":             {[]copy{{pixels: hd}, {pixels: sd, length: film}}, 0},
	} {
		for i, p := range plays(tc.copies) {
			if p != (i == tc.plays) {
				t.Errorf("%s: copy %d plays = %v", name, i, p)
			}
		}
	}
	if one := plays([]copy{{}}); !one[0] {
		t.Error("a title's only file does not play")
	}
}
