package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// The extra-copy trash (#2151 decision 37, step 10e). Remove extra copy moves a copy that never
// plays into TrashDir/<date>/ under its media folder: the same share, so the move is a rename,
// and a mistake is undone by moving the file back. A complete walk of the folder empties each
// day's folder TrashKeep after that day. TrashDir holds a .ignore, so walks and watches skip it.

const (
	// TrashDir is the trash folder at the top of each media folder.
	TrashDir = ".loomarr-trash"
	// TrashKeep is how long a removed copy waits before the trash is emptied of it.
	TrashKeep = 30 * 24 * time.Hour
)

var (
	// ErrPlays refuses to remove the copy that plays.
	ErrPlays = errors.New("library: that copy is the one that plays")
	// ErrTrash is a copy that could not be moved to the trash: the folder is read-only, or the
	// share refused.
	ErrTrash = errors.New("library: couldn't move the copy to the trash")
)

// RemoveExtraCopy is "Remove extra copy": path, one of the title's files that never plays, moves to
// its media folder's trash and leaves the title, and the folder is passed over. store.ErrNotFound
// when the title has no such present file; ErrPlays when it is the copy that plays.
func (l *Library) RemoveExtraCopy(ctx context.Context, id, path string) error {
	d, err := l.Item(ctx, id)
	if err != nil {
		return err
	}
	for _, f := range d.Files {
		if f.Path != path {
			continue
		}
		if !f.Extra {
			return ErrPlays
		}
		now := l.now()
		if err := moveToTrash(f.Root, f.Path, now); err != nil {
			return fmt.Errorf("%w: %w", ErrTrash, err)
		}
		// Loomarr moved it, so it is gone now: no walk has to find it missing, and no guard
		// against mass removals holds it.
		if err := l.ItemStore.RemoveLibraryFile(ctx, f.Path, now); err != nil {
			return err
		}
		l.pass([]librarystore.LibraryFilePath{{Root: f.Root, Path: f.Path}})
		return nil
	}
	return store.ErrNotFound
}

// moveToTrash moves path, a file under root, to root's trash for the day of now, keeping its
// place under root so it can be put back.
func moveToTrash(root, path string, now time.Time) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("%s is not under %s", path, root)
	}
	trash := filepath.Join(root, TrashDir)
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return err
	}
	ignore := filepath.Join(trash, ".ignore")
	if _, err := os.Stat(ignore); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(ignore, nil, 0o644); err != nil {
			return err
		}
	}
	dst := filepath.Join(trash, now.UTC().Format(time.DateOnly), rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("%s is already in the trash", dst)
	}
	return os.Rename(path, dst)
}

// emptyTrash deletes each day's folder in root's trash once TrashKeep has passed since that day.
// A folder that isn't a date is left alone: Loomarr didn't make it.
func emptyTrash(root string, now time.Time) error {
	trash := filepath.Join(root, TrashDir)
	entries, err := os.ReadDir(trash)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		day, err := time.Parse(time.DateOnly, e.Name())
		if !e.IsDir() || err != nil || now.Sub(day) < TrashKeep {
			continue
		}
		errs = append(errs, os.RemoveAll(filepath.Join(trash, e.Name())))
	}
	return errors.Join(errs...)
}
