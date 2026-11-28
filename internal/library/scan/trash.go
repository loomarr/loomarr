package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// The extra-copy trash (#2151 decision 37, step 10e). Remove extra copy moves a copy that never
// plays into TrashDir/<date>/ under its media folder: the same share, so the move is a rename,
// and a mistake is undone by moving the file back (trashundo.go). A complete walk of the folder empties each
// day's folder TrashKeep after that day. TrashDir holds a .ignore, so walks and watches skip it.

const (
	// TrashDir is the trash folder at the top of each media folder.
	TrashDir = ".loomarr-trash"
	// TrashKeep is how long a removed copy waits before the trash is emptied of it.
	TrashKeep = 30 * 24 * time.Hour
)

var (
	// ErrPlays refuses to remove a file that is not an extra copy: the copy that plays, one that
	// can't be played, or one still being read or waiting.
	ErrPlays = errors.New("library: that copy isn't an extra")
	// ErrPlayingCopyUnseen refuses to remove an extra copy while no copy that plays in its place
	// is found on disk: it is gone, or its share does not answer.
	ErrPlayingCopyUnseen = errors.New("library: the copy that plays is not on disk now")
	// ErrTrash is a copy that could not be moved to the trash: the folder is read-only, or the
	// share refused.
	ErrTrash = errors.New("library: couldn't move the copy to the trash")
)

// RemoveExtraCopy is "Remove extra copy": path, one of the title's files that never plays, moves to
// its media folder's trash and leaves the title, and the folder is passed over. It answers with the
// token that RestoreExtraCopy takes to undo it. store.ErrNotFound when the title has no such present
// file; ErrPlays when it is not an extra copy (librarystore.LibraryCopyExtra); ErrPlayingCopyUnseen
// when no copy that plays in its place is on disk now. One removal runs at a time, from reading the title to marking the file gone,
// so a second sees what the first did.
func (l *Library) RemoveExtraCopy(ctx context.Context, id, path string) (string, error) {
	l.removing.Lock()
	defer l.removing.Unlock()
	d, err := l.Item(ctx, id, false)
	if err != nil {
		return "", err
	}
	for _, f := range d.Files {
		if f.Path != path {
			continue
		}
		// Only an extra copy goes, by the store's rule. The list can keep a file that is gone (a
		// held removal, an arr not answering), so a copy that plays in its place must be on disk
		// too.
		if f.State != librarystore.LibraryCopyExtra || len(f.InPlaceOf) == 0 {
			return "", ErrPlays
		}
		if !slices.ContainsFunc(f.InPlaceOf, func(p string) bool { return l.disk().File(ctx, p) }) {
			return "", ErrPlayingCopyUnseen
		}
		now := l.now()
		moved, err := moveToTrash(f.Root, f.Path, now)
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrTrash, err)
		}
		// Loomarr moved it, so it is gone now: no walk has to find it missing, and no guard
		// against mass removals holds it.
		if err := l.ItemStore.RemoveLibraryFile(ctx, f.Path, now); err != nil {
			return "", err
		}
		l.pass([]librarystore.LibraryFilePath{{Root: f.Root, Path: f.Path}})
		moved.Item = id
		return l.undoToken(moved)
	}
	return "", store.ErrNotFound
}

// disk is the folders' Disk; the real filesystem when the Library has none.
func (l *Library) disk() Disk {
	if l.Roots != nil && l.Disk != nil {
		return l.Disk
	}
	return OSDisk{}
}

// moveToTrash moves path, a file under root, to root's trash for the day of now, keeping its
// place under root so it can be put back, and says where it went.
func moveToTrash(root, path string, now time.Time) (trashed, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return trashed{}, fmt.Errorf("%s is not under %s", path, root)
	}
	trash := filepath.Join(root, TrashDir)
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return trashed{}, err
	}
	ignore := filepath.Join(trash, ".ignore")
	if _, err := os.Stat(ignore); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(ignore, nil, 0o644); err != nil {
			return trashed{}, err
		}
	}
	moved := trashed{Root: root, Rel: rel, Day: now.UTC().Format(time.DateOnly)}
	dst := filepath.Join(trash, moved.Day, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return trashed{}, err
	}
	if _, err := os.Lstat(dst); err == nil {
		return trashed{}, fmt.Errorf("%s is already in the trash", dst)
	}
	return moved, os.Rename(path, dst)
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
