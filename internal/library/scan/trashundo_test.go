package scan

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Undo of Remove extra copy: the file goes back where it was and the next read lists it as the
// extra copy again. A token that was not signed for this title by this process moves nothing, and
// neither does a restore whose file is gone from the trash or whose old place is taken.
func TestRestoreExtraCopyPutsItBack(t *testing.T) {
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
	lib := &Library{ItemStore: h.st, Inventory: h.inv}
	ctx := context.Background()

	remove := func() (undo, trashedPath string) {
		t.Helper()
		undo, err := lib.RemoveExtraCopy(ctx, id, soft)
		if err != nil {
			t.Fatal(err)
		}
		found, err := filepath.Glob(filepath.Join(h.root, TrashDir, "*", "*", "Foxtrot DVD.mkv"))
		if err != nil || len(found) != 1 {
			t.Fatalf("trashed copies = %v, %v; want one", found, err)
		}
		return undo, found[0]
	}
	exists := func(path string) bool {
		_, err := os.Lstat(path)
		return err == nil
	}

	undo, inTrash := remove()
	h.run()

	payload, sig, _ := strings.Cut(undo, ".")
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatal(err)
	}
	escaped := base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(raw), "Foxtrot DVD.mkv", "../../outside.mkv", 1)))
	for name, try := range map[string]struct {
		lib       *Library
		id, token string
	}{
		"not a token":           {lib, id, "Foxtrot DVD.mkv"},
		"a rewritten path":      {lib, id, escaped + "." + sig},
		"no signature":          {lib, id, payload + "."},
		"another title's":       {lib, id + "0", undo},
		"signed before restart": {&Library{ItemStore: h.st, Inventory: h.inv}, id, undo},
	} {
		if err := try.lib.RestoreExtraCopy(ctx, try.id, try.token); !errors.Is(err, ErrUndoToken) {
			t.Errorf("%s = %v, want ErrUndoToken", name, err)
		}
	}
	if !exists(inTrash) || exists(soft) {
		t.Fatal("a refused token moved the file")
	}

	// Something at the old place is never overwritten.
	if err := os.WriteFile(soft, []byte("newer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := lib.RestoreExtraCopy(ctx, id, undo); !errors.Is(err, ErrPathTaken) {
		t.Fatalf("restore onto a file = %v, want ErrPathTaken", err)
	}
	if got, err := os.ReadFile(soft); err != nil || string(got) != "newer" || !exists(inTrash) {
		t.Fatalf("after the refusal: %q, %v, in trash %v", got, err, exists(inTrash))
	}
	if err := os.Remove(soft); err != nil {
		t.Fatal(err)
	}

	// The folder it was in may have gone meanwhile; it is made again.
	if err := os.Rename(filepath.Dir(soft), filepath.Dir(soft)+" moved"); err != nil {
		t.Fatal(err)
	}
	if err := lib.RestoreExtraCopy(ctx, id, undo); err != nil {
		t.Fatal(err)
	}
	if !exists(soft) || exists(inTrash) {
		t.Fatal("the file is not back where it was")
	}
	if err := os.Rename(filepath.Join(filepath.Dir(soft)+" moved", "Foxtrot.mkv"), filepath.Join(filepath.Dir(soft), "Foxtrot.mkv")); err != nil {
		t.Fatal(err)
	}
	h.run()
	d, err := lib.Item(ctx, id, false)
	if err != nil || len(d.Files) != 2 {
		t.Fatalf("title after restore = %+v, %v; want both files", d.Files, err)
	}
	for _, f := range d.Files {
		if (f.State == librarystore.LibraryCopyExtra) != (f.Path == soft) {
			t.Errorf("%s state = %s", f.Path, f.State)
		}
	}
	if err := lib.RestoreExtraCopy(ctx, id, undo); !errors.Is(err, ErrTrashGone) {
		t.Errorf("a second restore = %v, want ErrTrashGone", err)
	}

	undo, inTrash = remove()
	if err := os.Remove(inTrash); err != nil {
		t.Fatal(err)
	}
	if err := lib.RestoreExtraCopy(ctx, id, undo); !errors.Is(err, ErrTrashGone) {
		t.Errorf("restore of an emptied copy = %v, want ErrTrashGone", err)
	}
}
