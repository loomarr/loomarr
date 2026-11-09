package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store"
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
	d, err := lib.Item(ctx, id)
	if err != nil || len(d.Files) != 2 {
		t.Fatalf("title = %+v, %v; want one title with both files", d.Files, err)
	}
	for _, f := range d.Files {
		if f.Extra != (f.Path == soft) {
			t.Errorf("%s extra = %v", f.Path, f.Extra)
		}
	}

	if err := lib.RemoveExtraCopy(ctx, id, sharp); !errors.Is(err, ErrPlays) {
		t.Fatalf("removing the copy that plays = %v, want ErrPlays", err)
	}
	if err := lib.RemoveExtraCopy(ctx, id, "/elsewhere/Foxtrot.mkv"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("removing another title's file = %v, want ErrNotFound", err)
	}
	if err := lib.RemoveExtraCopy(ctx, id, soft); err != nil {
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
	if d, err = lib.Item(ctx, id); err != nil || len(d.Files) != 1 || d.Files[0].Path != sharp || d.Files[0].Extra {
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

// The file an arr lists plays, however sharp another copy is; without one, the sharper, then the
// bigger.
func TestTheCopyThatPlays(t *testing.T) {
	hd := &Picture{Width: 1920, Height: 1080}
	sd := &Picture{Width: 720, Height: 480}
	for name, tc := range map[string]struct {
		files []ItemFile
		plays int
	}{
		"arr's file": {[]ItemFile{{Picture: hd}, {Picture: sd, managed: true}}, 1},
		"sharper":    {[]ItemFile{{Picture: sd}, {Picture: hd}}, 1},
		"bigger":     {[]ItemFile{{LibraryFile: store.LibraryFile{Size: 9}}, {LibraryFile: store.LibraryFile{Size: 3}}}, 0},
	} {
		markExtras(tc.files)
		for i, f := range tc.files {
			if f.Extra != (i != tc.plays) {
				t.Errorf("%s: file %d extra = %v", name, i, f.Extra)
			}
		}
	}
	one := []ItemFile{{}}
	if markExtras(one); one[0].Extra {
		t.Error("a title's only file is an extra")
	}
}
