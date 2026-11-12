package scan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/loomarr/loomarr/internal/library/metadataprovider/localart"
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
	"backdrops": true, "sample": true, "special features": true, "bonus features": true,
}

// skippedDir reports whether a walk passes over a folder named name: a by-product, a recycle bin, a
// hidden folder, or (below the top of the root) an extras folder. A disc structure is not walked
// either, but is listed as a problem.
func skippedDir(name string, belowTop bool) bool {
	lower := strings.ToLower(name)
	return skippedDirs[lower] || strings.HasPrefix(lower, ".") || (belowTop && extrasDirs[lower])
}

// extrasSuffixes mark an extra by its file name, as in "Movie (2001)-trailer.mkv"; it is skipped.
var extrasSuffixes = []string{
	"-trailer", "-featurette", "-behindthescenes", "-deleted", "-interview", "-scene", "-short", "-sample", "-other",
}

// extrasStems are extras' whole names, as a release's "sample.mkv" or a folder's "trailer.mkv".
var extrasStems = map[string]bool{"sample": true, "trailer": true}

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
	// Sidecars are the file's own NFO, artwork and trickplay files, by path, so a changed sidecar
	// changes the file's identity digest without being read. A trickplay file's Stat is zero: only
	// its being there counts.
	Sidecars map[string]Stat
	// Sole is whether it is its folder's only video, so the folder's own sidecars are its.
	Sole bool
	// Problem is why the path is not imported, "" for a media file.
	Problem string
	// Top is which of the root's top-level entries the file is under, counted as the walk lists
	// them, so the import can tell how many entries' files it has written.
	Top int
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
// is slow from one that is stuck. report, when not nil, hears each folder listed and each
// top-level entry finished.
func walkRoot(ctx context.Context, root string, steps *atomic.Int64, report *walkReport) (Walk, error) {
	w := newWalker(root, steps)
	w.report = report
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
		if skippedDir(seg, i > 0) || discDirs[strings.ToLower(seg)] {
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
	// report hears a whole root's walk go, for the import's progress; nil hears nothing.
	report *walkReport
	// top is the root's top-level entry being walked.
	top int
	out Walk
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
	// The folder is listed, and its files recorded, under its one canonical path, not the alias
	// that happened to reach it first.
	if canonical, ok := canonicalPath(w.root, w.realRoot, real); ok {
		dir = canonical
	}
	w.report.listing(w.rel(dir))
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
		switch {
		case isNFO(e.Name()) || localart.IsImage(e.Name()):
			if st, ok := w.stat(filepath.Join(dir, e.Name()), e); ok {
				nfos[filepath.Join(dir, e.Name())] = st
			}
		case (isTrickplay(e.Name()) || externalTrackKind(e.Name()) != "") && e.Type().IsRegular():
			// Referenced, never read: whether it is there is all that counts, so it costs no stat.
			nfos[filepath.Join(dir, e.Name())] = Stat{}
		case isEDL(e.Name()):
			if st, ok := w.stat(filepath.Join(dir, e.Name()), e); ok {
				nfos[filepath.Join(dir, e.Name())] = st
			}
		}
	}
	w.nfo[dir], w.videos[dir] = nfos, videos
	defer delete(w.nfo, dir)
	defer delete(w.videos, dir)

	top := dir == w.root
	if top {
		w.report.counted(len(entries))
	}
	for i, e := range entries {
		if top {
			// The entries before this one are listed.
			w.report.listed(i)
			w.top = i
		}
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
			// A file link to a file the walk records itself would record it twice; the target is
			// the one. A target in a folder the walk skips is reached only through the link.
			if canonical, ok := canonicalPath(w.root, w.realRoot, target); !isDir && ok && !w.within(filepath.Dir(canonical)) {
				continue
			}
		}
		switch {
		case isDir && skippedDir(e.Name(), w.rel(dir) != "."):
		case isDir && discDirs[lower]:
			w.problem(path, Stat{}, ProblemDiscImage)
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
				Sidecars: w.sidecars(dir, e.Name()), Sole: w.videos[dir] <= 1, Top: w.top,
			})
		}
	}
	return nil
}

// walkReport tells the Tracker how one root goes through step 1 of an import: the folder it is on,
// named from the root's own folder name down ("TV / Night Ferry / Season 2"), and how many of the
// root's top-level entries are finished. An entry is finished in two halves, once the walk has
// listed it and once the import has written its files, so the count moves through both: over a
// share either can take minutes. The walk runs on its own goroutine, which a walk given up on
// outlives, so it is locked, and close ends it.
type walkReport struct {
	tracker *Tracker
	// name is the root's folder name; entries is how many top-level entries the import counted.
	name    string
	entries int

	mu sync.Mutex
	// walked and written are how many entries the walk has listed and the import has written.
	walked, written int
	// reported is how many entries the Tracker was told are finished: half of walked and written.
	reported int
	closed   bool
}

// listing names the folder the walk lists, by its path relative to the root.
func (r *walkReport) listing(rel string) {
	if r == nil {
		return
	}
	current := r.name
	if rel != "." {
		current += " / " + strings.ReplaceAll(rel, "/", " / ")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.tracker.Current(StepWalking, current)
	}
}

// relDir is the folder of a path relative to the root, "." for a file in the root itself.
func relDir(rel string) string {
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return "."
}

// counted is the root's top-level entries as the walk lists them. The count made before the walk
// can come back empty (a listing late past readableTimeout) or be stale (an entry added or removed
// since), so step 1's total takes the walk's count: a root counted as nothing would otherwise read
// full while every one of its files is still to come.
func (r *walkReport) counted(n int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || n == r.entries {
		return
	}
	r.tracker.Grow(StepWalking, n-r.entries)
	r.entries = n
}

// listed reports n of the root's top-level entries listed by the walk.
func (r *walkReport) listed(n int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.walked = max(r.walked, min(n, r.entries))
	r.tell()
}

// writing begins the import's writes: the walk is over, and what is left goes at the writes' rate,
// so the time left is worked out from here.
func (r *walkReport) writing() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.walked = r.entries
		r.tell()
		r.tracker.Rebase(StepWalking)
	}
}

// wrote reports the files of n of the root's top-level entries written.
func (r *walkReport) wrote(n int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.written = max(r.written, min(n, r.entries))
	r.tell()
}

// tell reports to the Tracker the entries finished since it was last told: half of those listed
// and those written, never more than were counted. Called with mu held.
func (r *walkReport) tell() {
	if r.closed {
		return
	}
	if n := (r.walked + r.written) / 2; n > r.reported {
		r.tracker.Advance(StepWalking, n-r.reported)
		r.reported = n
	}
}

// close finishes every counted entry, however the root's pass ended, and stops hearing it.
func (r *walkReport) close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.walked, r.written = r.entries, r.entries
	r.tell()
	r.closed = true
}

// topEntries counts root's top-level entries within readableTimeout; 0 when it cannot be listed
// in time. A mount whose server went away can block a listing for good, so one that is late is
// left to end on its own.
func topEntries(ctx context.Context, root string) int {
	ctx, cancel := context.WithTimeout(ctx, readableTimeout)
	defer cancel()
	n := make(chan int, 1)
	go func() {
		entries, _ := os.ReadDir(root)
		n <- len(entries)
	}()
	select {
	case c := <-n:
		return c
	case <-ctx.Done():
		return 0
	}
}

// canonicalPath is real, a resolved path, as the root names it: the root joined with real's place
// under the root's own real path. Whatever path reached a folder or file, through a symlink or
// not, it is recorded under this one, so a full walk and a folder pass agree (#3227). ok is false
// when real lies outside the root.
func canonicalPath(root, realRoot, real string) (string, bool) {
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Join(root, rel), true
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
	w.out.Files = append(w.out.Files, File{Path: path, Rel: w.rel(path), Stat: st, Problem: problem, Top: w.top})
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
		case isTrickplay(path):
			// A trickplay file is one video's, by name: never the folder's.
			if base == stem || strings.HasPrefix(base, stem+"-") {
				out[path] = st
			}
		case externalTrackKind(path) != "" || isEDL(path):
			// So is an external track ("<name>.en.forced.srt") or an .edl.
			if base == stem || strings.HasPrefix(base, stem+".") {
				out[path] = st
			}
		case strings.HasPrefix(base, stem+"-") || sole:
			out[path] = st
		}
	}
	return out
}

func isNFO(name string) bool { return strings.EqualFold(filepath.Ext(name), ".nfo") }

// isTrickplay reports whether name is a trickplay file: a Roku BIF of seek thumbnails, which Emby
// writes beside the video as "<name>.bif" (decision 61).
func isTrickplay(name string) bool { return strings.EqualFold(filepath.Ext(name), ".bif") }

// isEDL reports whether name is an edit decision list: the spans a player skips or mutes, or marks
// as scenes or commercial breaks (decision 61).
func isEDL(name string) bool { return strings.EqualFold(filepath.Ext(name), ".edl") }

// externalCodecs are the external track files the walk notes, by extension: each one's kind and
// codec as ffprobe names it ("" where the extension does not say).
var externalCodecs = map[string][2]string{
	".srt": {"subtitle", "subrip"}, ".ass": {"subtitle", "ass"}, ".ssa": {"subtitle", "ssa"},
	".vtt": {"subtitle", "webvtt"}, ".sup": {"subtitle", "hdmv_pgs_subtitle"}, ".sub": {"subtitle", "dvd_subtitle"},
	".mka": {"audio", ""}, ".ac3": {"audio", "ac3"}, ".eac3": {"audio", "eac3"}, ".dts": {"audio", "dts"},
	".aac": {"audio", "aac"}, ".flac": {"audio", "flac"},
}

// externalTrackKind is "subtitle" or "audio" for an external track file, "" for anything else.
func externalTrackKind(name string) string {
	return externalCodecs[strings.ToLower(filepath.Ext(name))][0]
}

// edl is the file's .edl, "" for none.
func (f File) edl() string {
	for path := range f.Sidecars {
		if isEDL(path) {
			return path
		}
	}
	return ""
}

// trickplay is the file's trickplay files, by path, in order.
func (f File) trickplay() []string {
	var out []string
	for path := range f.Sidecars {
		if isTrickplay(path) {
			out = append(out, path)
		}
	}
	slices.Sort(out)
	return out
}

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

// extraName reports whether a video file's lower-cased name marks it as no title: an extra, a
// sample, or a hidden file such as a Mac's "._" shadow of a real one.
func extraName(lowerName string) bool {
	stem := strings.TrimSuffix(lowerName, filepath.Ext(lowerName))
	if strings.HasPrefix(lowerName, ".") || extrasStems[stem] || strings.HasPrefix(stem, "sample-") {
		return true
	}
	for _, suffix := range extrasSuffixes {
		if strings.HasSuffix(stem, suffix) {
			return true
		}
	}
	return false
}
