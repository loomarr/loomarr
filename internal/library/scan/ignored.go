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
	roots, err := l.rootPaths(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]IgnoredTitle, 0, len(titles))
	for _, t := range titles {
		out = append(out, ignoredTitle(t, files[t.ID], roots))
	}
	return out, total, nil
}

// rootPaths is every media folder, for telling folder labels apart.
func (l *Library) rootPaths(ctx context.Context) ([]string, error) {
	known, err := l.Store.LibraryRoots(ctx)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(known.Roots))
	for _, r := range known.Roots {
		roots = append(roots, r.Path)
	}
	return roots, nil
}

// ignoredTitle names t by what is on disk; one with no present file keeps its title and no folder.
func ignoredTitle(t librarystore.LibraryIgnoredTitle, files []librarystore.LibraryFile, roots []string) IgnoredTitle {
	out := IgnoredTitle{ID: t.ID, Kind: t.Kind, Name: t.Name, IgnoredAt: t.IgnoredAt}
	if name, folder, ok := onDisk(t.Kind, t.Origin, files, roots); ok {
		out.Folder = folder
		if name != "" {
			out.Name = name
		}
	}
	return out
}

// onDisk names a title by its present files, in path order, under its origin's media folder; a
// title with none left there is named by the files it has. It says the movie's file name or the
// show's folder name ("" for a show with no folder of its own), the folder that sits in, and
// whether there is a file at all.
//
// A show's origin is a digest of its folder, and a show joined from several folders has files
// outside it, so every file's ancestors are tried. The digest is of one folder, so every match is
// the same folder. A show with no folder of its own matches none: it is under the folder of its
// first file.
func onDisk(kind string, origin librarystore.LibraryOrigin, files []librarystore.LibraryFile, roots []string) (string, string, bool) {
	own := slices.DeleteFunc(slices.Clone(files), func(f librarystore.LibraryFile) bool {
		return Authority(f.Root) != inventory.AuthorityID(origin.Authority)
	})
	if len(own) == 0 {
		own = files
	}
	if len(own) == 0 {
		return "", "", false
	}
	root := own[0].Root
	label := rootLabel(root, roots)
	within := func(f librarystore.LibraryFile) string {
		return strings.TrimPrefix(filepath.ToSlash(f.Path), strings.TrimSuffix(filepath.ToSlash(root), "/")+"/")
	}
	first := within(own[0])
	folder := path.Join(label, path.Dir(first))
	if kind != "series" {
		return path.Base(first), folder, true
	}
	for _, f := range own {
		for dir := path.Dir(within(f)); f.Root == root && dir != "." && dir != "/"; dir = path.Dir(dir) {
			if seriesOrigin(root, dir).ExternalItemID == origin.Key {
				return path.Base(dir), path.Join(label, path.Dir(dir)), true
			}
		}
	}
	return "", folder, true
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
