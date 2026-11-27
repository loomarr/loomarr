package scan

import (
	"context"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// IgnoredTitle is a title the maintainer ignored, named by what is on disk.
type IgnoredTitle struct {
	ID string
	// Kind is movie or series.
	Kind string
	// Name is a movie's file name or a show's folder name.
	Name string
	// Folder is where it sits, from its media folder's name down.
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
	known, err := l.Store.LibraryRoots(ctx)
	if err != nil {
		return nil, 0, err
	}
	roots := make([]string, 0, len(known.Roots))
	for _, r := range known.Roots {
		roots = append(roots, r.Path)
	}
	out := make([]IgnoredTitle, 0, len(titles))
	for _, t := range titles {
		out = append(out, ignoredTitle(t, files[t.ID], roots))
	}
	return out, total, nil
}

// ignoredTitle names t by its present files, in path order, under its origin's media folder; a
// title with none left there is named by the files it has.
//
// A show's origin is a digest of its folder, and a show joined from several folders has files
// outside it, so every file's ancestors are tried. The digest is of one folder, so every match is
// the same folder. A show with no folder of its own matches none: it keeps its title, under the
// folder of its first file.
func ignoredTitle(t librarystore.LibraryIgnoredTitle, files []librarystore.LibraryFile, roots []string) IgnoredTitle {
	out := IgnoredTitle{ID: t.ID, Kind: t.Kind, Name: t.Name, IgnoredAt: t.IgnoredAt}
	own := slices.DeleteFunc(slices.Clone(files), func(f librarystore.LibraryFile) bool {
		return Authority(f.Root) != inventory.AuthorityID(t.Origin.Authority)
	})
	if len(own) == 0 {
		own = files
	}
	if len(own) == 0 {
		return out
	}
	root := own[0].Root
	label := rootLabel(root, roots)
	within := func(f librarystore.LibraryFile) string {
		return strings.TrimPrefix(filepath.ToSlash(f.Path), strings.TrimSuffix(filepath.ToSlash(root), "/")+"/")
	}
	first := within(own[0])
	out.Folder = path.Join(label, path.Dir(first))
	if t.Kind != "series" {
		out.Name = path.Base(first)
		return out
	}
	for _, f := range own {
		for dir := path.Dir(within(f)); f.Root == root && dir != "." && dir != "/"; dir = path.Dir(dir) {
			if seriesOrigin(root, dir).ExternalItemID == t.Origin.Key {
				out.Name, out.Folder = path.Base(dir), path.Join(label, path.Dir(dir))
				return out
			}
		}
	}
	return out
}

// rootLabel is the name Settings → Library shows for a media folder (the web's folderName), or its
// whole path when another media folder has the same name.
func rootLabel(root string, roots []string) string {
	name := rootName(root)
	if slices.ContainsFunc(roots, func(other string) bool { return other != root && rootName(other) == name }) {
		return root
	}
	return name
}

// rootName is a media folder's last folder name, capitalized: "tv" and other names of one or two
// letters are all capitals.
func rootName(root string) string {
	name := filepath.Base(root)
	if utf8.RuneCountInString(name) <= 2 {
		return strings.ToUpper(name)
	}
	first, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(first)) + name[size:]
}
