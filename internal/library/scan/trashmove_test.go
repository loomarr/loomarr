package scan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A restore's move never replaces a file, even one written after the caller looked, and leaves the
// file in exactly one place however it ends, with hard links or without them.
func TestMoveNoReplace(t *testing.T) {
	noLinks := func(string, string) error { return &os.LinkError{Op: "link", Err: syscall.EPERM} }
	for name, link := range map[string]func(string, string) error{"hard links": os.Link, "no hard links": noLinks} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
			write(t, src, "trashed")
			write(t, dst, "newer")
			if err := moveNoReplace(src, dst, link, os.Remove); !errors.Is(err, fs.ErrExist) {
				t.Errorf("onto a file = %v, want fs.ErrExist", err)
			}
			holds(t, src, "trashed")
			holds(t, dst, "newer")

			if err := os.Remove(dst); err != nil {
				t.Fatal(err)
			}
			write(t, src, "trashed")
			if err := moveNoReplace(src, dst, link, os.Remove); err != nil {
				t.Fatal(err)
			}
			holds(t, dst, "trashed")
			absent(t, src)
		})
	}

	// The link is made but the old name can't be dropped: the new one is taken away again.
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	write(t, src, "trashed")
	stuck := func(path string) error {
		if path == src {
			return fs.ErrPermission
		}
		return os.Remove(path)
	}
	if err := moveNoReplace(src, dst, os.Link, stuck); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("with a stuck source = %v, want ErrPermission", err)
	}
	holds(t, src, "trashed")
	absent(t, dst)

	// A symlink moves as itself: its target is neither linked nor moved.
	target, link := filepath.Join(dir, "target"), filepath.Join(dir, "link")
	write(t, target, "target")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := moveNoReplace(link, dst, os.Link, os.Remove); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(dst); err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("moved symlink = %v, %v; want a symlink", info, err)
	}
	absent(t, link)
}

// A folder swapped for a symlink that leaves the media folder is never written through.
func TestRestoreStaysUnderTheMediaFolder(t *testing.T) {
	for _, rel := range []string{"Title/copy.mkv", "Title/Extras/copy.mkv"} {
		root, outside := t.TempDir(), t.TempDir()
		lib := &Library{}
		moved := trashed{Item: "1", Root: root, Rel: rel, Day: "2026-11-27"}
		inTrash := filepath.Join(root, TrashDir, moved.Day, rel)
		if err := os.MkdirAll(filepath.Dir(inTrash), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, inTrash, "trashed")
		if err := os.Symlink(outside, filepath.Join(root, "Title")); err != nil {
			t.Fatal(err)
		}
		undo, err := lib.undoToken(moved)
		if err != nil {
			t.Fatal(err)
		}
		if err := lib.RestoreExtraCopy(context.Background(), "1", undo); !errors.Is(err, ErrRestore) {
			t.Errorf("%s through a symlink = %v, want ErrRestore", rel, err)
		}
		holds(t, inTrash, "trashed")
		if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
			t.Errorf("%s wrote outside the media folder: %v, %v", rel, entries, err)
		}
	}
}

// A signature alone is not enough: a signed path that leaves the media folder, or a day that is no
// date, is refused.
func TestSignedUndoTokenIsStillChecked(t *testing.T) {
	lib := &Library{}
	good := trashed{Item: "1", Root: "/media", Rel: "Title/copy.mkv", Day: "2026-11-27"}
	for name, change := range map[string]func(*trashed){
		"climbing path": func(v *trashed) { v.Rel = "../copy.mkv" },
		"absolute path": func(v *trashed) { v.Rel = "/etc/copy.mkv" },
		"empty path":    func(v *trashed) { v.Rel = "" },
		"climbing day":  func(v *trashed) { v.Day = "../.." },
		"no day":        func(v *trashed) { v.Day = "" },
	} {
		bad := good
		change(&bad)
		undo, err := lib.undoToken(bad)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lib.readUndoToken("1", undo); !errors.Is(err, ErrUndoToken) {
			t.Errorf("%s = %v, want ErrUndoToken", name, err)
		}
	}
	undo, err := lib.undoToken(good)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := lib.readUndoToken("1", undo); err != nil || got != good {
		t.Errorf("a good token = %+v, %v", got, err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func holds(t *testing.T, path, body string) {
	t.Helper()
	if got, err := os.ReadFile(path); err != nil || string(got) != body {
		t.Errorf("%s holds %q, %v; want %q", path, got, err, body)
	}
}

func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s is still there: %v", path, err)
	}
}
