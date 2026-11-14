package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Removals: what a pass no longer finds. A file moved or renamed is the same bytes at a new path;
// one gone is held until the household confirms it, so a share that stops answering removes
// nothing on its own.

// movedFrom is each file this pass no longer finds whose header was read, by its size and
// modification time: a new path with the same pair is the same bytes renamed or moved, as Sonarr
// and Radarr do on every rename. A pair two vanished files share names neither.
func movedFrom(prev map[string]librarystore.LibraryFile, found map[string]bool, inScope func(string) bool) map[Stat]librarystore.LibraryFile {
	out := map[Stat]librarystore.LibraryFile{}
	shared := map[Stat]bool{}
	for path, r := range prev {
		if found[path] || !inScope(path) || !r.MissingAt.IsZero() || r.Problem != "" || r.Header != librarystore.LibraryHeaderRead {
			continue
		}
		key := Stat{Size: r.Size, ModifiedNs: r.ModifiedNs}
		if _, ok := out[key]; ok {
			shared[key] = true
		}
		out[key] = r
	}
	for key := range shared {
		delete(out, key)
	}
	return out
}

// removals is every present file in the walk's scope that it did not find, and how many
// importable files the root held. A problem file that is gone is a removal too, but never counts
// toward a hold.
func removals(prev map[string]librarystore.LibraryFile, found map[string]bool, inScope func(string) bool) ([]librarystore.LibraryFile, int) {
	var removed []librarystore.LibraryFile
	present := 0
	for path, r := range prev {
		if !r.MissingAt.IsZero() {
			continue
		}
		if r.Problem == "" {
			present++
		}
		if !found[path] && inScope(path) {
			removed = append(removed, r)
		}
	}
	return removed, present
}

// under reports whether path is one of dirs or inside one.
func under(path string, dirs []string) bool {
	for _, d := range dirs {
		if path == d || strings.HasPrefix(path, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// existingFolders is each folder in dirs, or, for one that is gone, its nearest ancestor inside
// root that is not: a folder deleted with everything in it is a change to the folder above.
func existingFolders(root string, dirs []string) []string {
	out := []string{}
	realRoot, rootErr := filepath.EvalSymlinks(root)
	for _, d := range dirs {
		d = filepath.Clean(d)
		for d != root && under(d, []string{root}) {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				break
			}
			d = filepath.Dir(d)
		}
		if !under(d, []string{root}) {
			d = root
		}
		// A folder named through a symlink is scoped as the walk records it: its canonical path,
		// so the pass's removals cover the files it lists (#3227).
		if real, err := filepath.EvalSymlinks(d); err == nil && rootErr == nil {
			if canonical, ok := canonicalPath(root, realRoot, real); ok {
				d = canonical
			}
		}
		out = append(out, d)
	}
	return out
}

// ConfirmRemovals records the maintainer's confirmation of a root's held removal. The next pass
// of that root applies its removal however large. It never waits for a running pass.
func (im *Importer) ConfirmRemovals(ctx context.Context, root string) error {
	rows, err := im.Store.LibraryFiles(ctx, root)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Held {
			im.mu.Lock()
			if im.confirmed == nil {
				im.confirmed = map[string]bool{}
			}
			im.confirmed[root] = true
			im.mu.Unlock()
			return nil
		}
	}
	return ErrNothingHeld
}

// takeConfirmed reports and clears a root's pending confirmation.
func (im *Importer) takeConfirmed(root string) bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	ok := im.confirmed[root]
	delete(im.confirmed, root)
	return ok
}

// ErrNothingHeld is a confirmation for a root with no held removal.
var ErrNothingHeld = errors.New("scan: no removal is waiting for this folder")

func (im *Importer) markUnseen(ctx context.Context, root string, seen map[inventory.OriginKey]bool, at time.Time) error {
	keys := make([]inventory.OriginKey, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return im.Inventory.MarkUnseen(ctx, Authority(root), at, keys)
}
