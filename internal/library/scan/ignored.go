package scan

import (
	"context"
	"path"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// IgnoredTitle is a title the maintainer ignored, named by what is on disk.
type IgnoredTitle struct {
	ID string
	// Kind is movie or series.
	Kind string
	// Name is a movie's file name or a show's folder name.
	Name string
	// Folder is where it sits, from its media folder's own name down.
	Folder    string
	IgnoredAt time.Time
}

// Ignored is a page of the ignored titles, newest first, and how many there are in all.
func (l *Library) Ignored(ctx context.Context, limit, offset int) ([]IgnoredTitle, int, error) {
	titles, total, err := l.NeedStore.LibraryIgnored(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(titles))
	for _, t := range titles {
		ids = append(ids, t.ID)
	}
	files, err := l.ItemStore.LibraryItemsFiles(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]IgnoredTitle, 0, len(titles))
	for _, t := range titles {
		out = append(out, ignoredTitle(t, files[t.ID]))
	}
	return out, total, nil
}

// ignoredTitle names t by its present files, in path order. A show's origin is a digest of its
// folder, so its folder is the one above a file that has that digest. A show with no folder of its
// own keeps its title.
func ignoredTitle(t librarystore.LibraryIgnoredTitle, files []librarystore.LibraryFile) IgnoredTitle {
	out := IgnoredTitle{ID: t.ID, Kind: t.Kind, Name: t.Name, IgnoredAt: t.IgnoredAt}
	if len(files) == 0 {
		return out
	}
	root := files[0].Root
	rel, err := filepath.Rel(root, files[0].Path)
	if err != nil {
		return out
	}
	rel = filepath.ToSlash(rel)
	at := func(dir string) string { return path.Join(filepath.Base(root), dir) }
	out.Folder = at(path.Dir(rel))
	if t.Kind != "series" {
		out.Name = path.Base(rel)
		return out
	}
	for dir := path.Dir(rel); dir != "."; dir = path.Dir(dir) {
		if seriesOrigin(root, dir).ExternalItemID == t.Origin.Key {
			out.Name, out.Folder = path.Base(dir), at(path.Dir(dir))
			break
		}
	}
	return out
}
