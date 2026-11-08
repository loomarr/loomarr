package scan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/loomarr/loomarr/internal/metadataprovider/localart"
)

// Problems a pass records for a path it does not import.
const (
	ProblemDiscImage        = "disc_image"
	ProblemIdentityConflict = "identity_conflict"
	// ProblemInvalid is a file whose facts the inventory refused.
	ProblemInvalid = "invalid_facts"
)

// videoExtensions are the containers the scanner imports.
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".ts": true, ".m2ts": true, ".mts": true,
	".mov": true, ".wmv": true, ".mpg": true, ".mpeg": true, ".webm": true,
}

// skippedDirs hold media-server and NAS by-products and recycle bins, never media. A ".trash-<uid>"
// folder is skipped by its prefix.
var skippedDirs = map[string]bool{
	"trickplay": true, "extrafanart": true, ".actors": true, "@eadir": true,
	".recycle.bin": true, "#recycle": true, "$recycle.bin": true, "lost+found": true,
}

// discDirs are disc structures, which are not imported but listed as problems.
var discDirs = map[string]bool{"video_ts": true, "bdmv": true}

// extrasDirs are the Kodi/Jellyfin folders that hold a title's extras and theme videos. Below the top
// of a root they are skipped: only movies and episodes become titles (0047). At the top they are
// titles' own folders, such as a show named "Shorts".
var extrasDirs = map[string]bool{
	"extras": true, "featurettes": true, "behind the scenes": true, "deleted scenes": true,
	"interviews": true, "scenes": true, "shorts": true, "trailers": true, "other": true, "samples": true,
	"backdrops": true,
}

// extrasSuffixes mark an extra by its file name, as in "Movie (2001)-trailer.mkv"; it is skipped.
var extrasSuffixes = []string{
	"-trailer", "-featurette", "-behindthescenes", "-deleted", "-interview", "-scene", "-short", "-sample", "-other",
}

// Stat is a file's size and modification time, which together are its revision.
type Stat struct {
	Size       int64
	ModifiedNs int64
}

// File is one media file, or one problem path, a walk found.
type File struct {
	// Path is absolute; Rel is relative to the root, with forward slashes.
	Path string
	Rel  string
	Stat Stat
	// Sidecars are the file's own NFO and artwork files, by path, so a changed sidecar changes the
	// file's identity digest without being read.
	Sidecars map[string]Stat
	// Sole is whether it is its folder's only video, so the folder's own sidecars are its.
	Sole bool
	// Problem is why the path is not imported, "" for a media file.
	Problem string
}

// Walk is one walk of one root.
type Walk struct {
	Files []File
	// Complete is false when any folder could not be read, so a missing file proves nothing.
	Complete bool
	// Skipped are paths the walk could not keep because their names are not UTF-8.
	Skipped []string
}

// walkRoot lists the media under root. It skips media-server by-products and folders holding an
// .ignore file, lists disc images as problems, and follows a symlink only when its target is
// inside the root.
// steps, when not nil, counts each folder listed and file read, so a caller can tell a walk that
// is slow from one that is stuck.
func walkRoot(ctx context.Context, root string, steps *atomic.Int64) (Walk, error) {
	w := newWalker(root, steps)
	real, err := filepath.EvalSymlinks(w.root)
	if err != nil {
		return Walk{}, err
	}
	w.realRoot = real
	if err := w.dir(ctx, w.root); err != nil {
		return Walk{}, err
	}
	w.out.Skipped = w.skipped
	return w.out, nil
}

// walkFolders lists the media under some folders of root, by the same rules as a walk of the whole
// root. A folder inside a skipped or extras folder is walked as that folder would be.
func walkFolders(ctx context.Context, root string, dirs []string, steps *atomic.Int64) (Walk, error) {
	w := newWalker(root, steps)
	real, err := filepath.EvalSymlinks(w.root)
	if err != nil {
		return Walk{}, err
	}
	w.realRoot = real
	for _, dir := range dirs {
		if w.within(dir) {
			continue
		}
		if err := w.dir(ctx, dir); err != nil {
			return Walk{}, err
		}
	}
	w.out.Skipped = w.skipped
	return w.out, nil
}

// within reports whether dir lies in a folder a walk skips: a by-product, a disc structure, an extras
// folder below the top of the root, or one holding an .ignore file above it.
func (w *walker) within(dir string) bool {
	rel := w.rel(dir)
	if rel == "." {
		return false
	}
	at := w.root
	for i, seg := range strings.Split(rel, "/") {
		if _, err := os.Stat(filepath.Join(at, ".ignore")); err == nil {
			return true
		}
		lower := strings.ToLower(seg)
		if skippedDirs[lower] || strings.HasPrefix(lower, ".trash-") || discDirs[lower] || seg == ".." ||
			(i > 0 && extrasDirs[lower]) {
			return true
		}
		at = filepath.Join(at, seg)
	}
	return false
}

type walker struct {
	root     string
	realRoot string
	visited  map[string]bool
	// nfo is each folder's NFO and artwork files, kept while the walk is under it.
	nfo map[string]map[string]Stat
	// videos counts each folder's videos, kept alongside nfo.
	videos map[string]int
	// steps counts folders listed and files read.
	steps *atomic.Int64
	out   Walk
	// skipped are paths whose names are not UTF-8.
	skipped []string
}

func newWalker(root string, steps *atomic.Int64) *walker {
	if steps == nil {
		steps = new(atomic.Int64)
	}
	w := &walker{
		root: filepath.Clean(root), visited: map[string]bool{},
		nfo: map[string]map[string]Stat{}, videos: map[string]int{}, steps: steps,
	}
	w.out.Complete = true
	return w
}

func (w *walker) dir(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// A folder reached twice, through a symlink inside the root, is walked once.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		w.out.Complete = false
		return nil
	}
	if w.visited[real] {
		return nil
	}
	w.visited[real] = true
	entries, err := os.ReadDir(dir)
	w.steps.Add(1)
	if err != nil {
		w.out.Complete = false
		return nil
	}
	nfos := map[string]Stat{}
	videos := 0
	for _, e := range entries {
		if e.Name() == ".ignore" {
			return nil
		}
		if e.IsDir() {
			continue
		}
		if lower := strings.ToLower(e.Name()); videoExtensions[filepath.Ext(lower)] && !extraName(lower) {
			videos++
		}
		if isNFO(e.Name()) || localart.IsImage(e.Name()) {
			if st, ok := w.stat(filepath.Join(dir, e.Name()), e); ok {
				nfos[filepath.Join(dir, e.Name())] = st
			}
		}
	}
	w.nfo[dir], w.videos[dir] = nfos, videos
	defer delete(w.nfo, dir)
	defer delete(w.videos, dir)

	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if !utf8.ValidString(e.Name()) {
			// The store keeps paths as text; a name that is not UTF-8 cannot be one.
			w.skipped = append(w.skipped, path)
			continue
		}
		lower := strings.ToLower(e.Name())
		isDir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 {
			target, ok := w.inside(path)
			if !ok {
				continue
			}
			info, err := os.Stat(target)
			if err != nil {
				continue
			}
			isDir = info.IsDir()
		}
		switch {
		case isDir && (skippedDirs[lower] || strings.HasPrefix(lower, ".trash-")):
		case isDir && discDirs[lower]:
			w.problem(path, Stat{}, ProblemDiscImage)
		case isDir && extrasDirs[lower] && w.rel(dir) != ".":
		case isDir:
			if err := w.dir(ctx, path); err != nil {
				return err
			}
		case filepath.Ext(lower) == ".iso" || filepath.Ext(lower) == ".img":
			st, _ := w.stat(path, e)
			w.problem(path, st, ProblemDiscImage)
		case videoExtensions[filepath.Ext(lower)] && !extraName(lower):
			st, ok := w.stat(path, e)
			if !ok {
				w.out.Complete = false
				continue
			}
			w.out.Files = append(w.out.Files, File{
				Path: path, Rel: w.rel(path), Stat: st,
				Sidecars: w.sidecars(dir, e.Name()), Sole: w.videos[dir] <= 1,
			})
		}
	}
	return nil
}

// inside resolves a symlink, reporting its target only when the target stays inside the root.
func (w *walker) inside(path string) (string, bool) {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(w.realRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return target, true
}

func (w *walker) stat(path string, e fs.DirEntry) (Stat, bool) {
	defer w.steps.Add(1)
	info, err := e.Info()
	if err == nil && info.Mode()&fs.ModeSymlink != 0 {
		info, err = os.Stat(path)
	}
	if err != nil || !info.Mode().IsRegular() {
		return Stat{}, false
	}
	return Stat{Size: info.Size(), ModifiedNs: info.ModTime().UnixNano()}, true
}

func (w *walker) problem(path string, st Stat, problem string) {
	w.out.Files = append(w.out.Files, File{Path: path, Rel: w.rel(path), Stat: st, Problem: problem})
}

func (w *walker) rel(path string) string {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// sidecars is the files that describe the media file named name in dir: its own <stem>.nfo and a
// movie folder's movie.nfo; its own "<stem>-poster.jpg" and kin; and, when it is the folder's only
// video, the folder's other images (poster.jpg …). A sibling's sidecars say nothing about it.
func (w *walker) sidecars(dir, name string) map[string]Stat {
	out := map[string]Stat{}
	stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	sole := w.videos[dir] <= 1
	for path, st := range w.nfo[dir] {
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		switch {
		case isNFO(path):
			if base == stem || base == "movie" {
				out[path] = st
			}
		case strings.HasPrefix(base, stem+"-") || sole:
			out[path] = st
		}
	}
	return out
}

func isNFO(name string) bool { return strings.EqualFold(filepath.Ext(name), ".nfo") }

// hasNFO reports whether the file has an NFO to read.
func (f File) hasNFO() bool {
	for path := range f.Sidecars {
		if isNFO(path) {
			return true
		}
	}
	return false
}

// art is the file's artwork sidecars, by name.
func (f File) art() []string {
	var names []string
	for path := range f.Sidecars {
		if localart.IsImage(path) {
			names = append(names, filepath.Base(path))
		}
	}
	return names
}

func extraName(lowerName string) bool {
	stem := strings.TrimSuffix(lowerName, filepath.Ext(lowerName))
	for _, suffix := range extrasSuffixes {
		if strings.HasSuffix(stem, suffix) {
			return true
		}
	}
	return false
}
