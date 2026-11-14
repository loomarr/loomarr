package scan

import (
	"context"
	"io/fs"
	"os"
	"path"
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
	// ProblemBrokenLink is a symlink whose target is gone: what it held can't be read.
	ProblemBrokenLink = "broken_link"
	// ProblemLinkOutside is a symlink to a file or folder outside the root, which a walk never
	// follows: the root is all the library reads.
	ProblemLinkOutside = "link_outside"
)

// walkProblem says whether a problem is one the walk finds, a path it never reads, rather than
// one a file read was refused with.
func walkProblem(problem string) bool {
	return problem == ProblemDiscImage || problem == ProblemBrokenLink || problem == ProblemLinkOutside
}

// videoExtensions are the containers the scanner imports: every one ffmpeg reads that a library
// keeps films and episodes in, old rips' among them (DivX, Flash, OGM, RealMedia, a lone VOB).
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".ts": true, ".m2ts": true, ".mts": true,
	".mov": true, ".wmv": true, ".mpg": true, ".mpeg": true, ".webm": true,
	".divx": true, ".flv": true, ".f4v": true, ".ogm": true, ".ogv": true, ".rmvb": true, ".rm": true,
	".vob": true, ".asf": true, ".3gp": true, ".m2v": true, ".xvid": true,
	// Windows Media Center's recordings, 3D Matroska, camcorders' MPEG and phones' MPEG-4.
	".wtv": true, ".dvr-ms": true, ".mk3d": true, ".mod": true, ".tod": true, ".mpeg4": true, ".3g2": true,
	// HDV camcorders' and satellite receivers' transport streams.
	".m2t": true, ".trp": true, ".tp": true,
}

// skippedDirs hold media-server and NAS by-products, snapshots and recycle bins, never media. A
// ".trash-<uid>" folder is skipped as hidden; a NAS's "@…" folders (QNAP's "@Recycle" and
// "@Recently-Snapshot", Synology's "@eaDir") and a downloader's unfinished "_UNPACK_…" and
// "_FAILED_…" folders by their prefixes (skippedPrefixes). A snapshot holds a second copy of the
// whole library.
var skippedDirs = map[string]bool{
	"trickplay": true, "extrafanart": true, ".actors": true,
	".recycle.bin": true, "#recycle": true, "$recycle.bin": true, "lost+found": true, "incomplete": true,
	"#snapshot": true, "system volume information": true, "plex versions": true,
	// A Mac's leavings on a share, Windows' older recycle bins, NZBGet's unpacking.
	"network trash folder": true, "temporary items": true, "recycler": true, "recycle bin": true,
	"intermediate": true,
}

// subsDirs are the folders a release keeps its subtitles in.
var subsDirs = map[string]bool{"subs": true, "subtitles": true, "subtitle": true, "sub": true}

// skippedPrefixes begin the names of folders skippedDirs skips by their prefix.
var skippedPrefixes = []string{".", "@", "~", "_unpack_", "_failed_"}

// discDirs are disc structures, which are not imported but listed as problems.
var discDirs = map[string]bool{"video_ts": true, "bdmv": true}

// skippedDir reports whether a walk passes over a folder named name in parent (relative to the
// root, "." its top): a by-product, a recycle bin, a download still coming, a hidden folder, or an
// extras folder in a title's own folder (titled, as holdsTitle says), which a season folder's
// "Specials" is (Emby's). An extras folder in a catch-all ("Movies/Shorts") or a folder of folders
// ("Movies/Kids/Shorts") is a collection of titles, not a title's extras. A disc structure is not
// walked either, but is listed as a problem.
func skippedDir(parent, name string, titled bool) bool {
	lower := strings.ToLower(name)
	// A NAS's own folders have no spaces ("@eaDir", ".Trash-1000"); "@Home Again (2019)" is a title's.
	prefixed := !strings.Contains(lower, " ") && slices.ContainsFunc(skippedPrefixes, func(p string) bool { return strings.HasPrefix(lower, p) })
	// A film's own "Specials" ("Movies/Ashwood (2015)/Specials/") are its extras, where a show's
	// are its season 0; and a subtitles' folder holds no title, whatever a release put in it.
	filmSpecials := lower == "specials" && parent != "." && amongFilms(parent) && !shelfAt(parent)
	// A films' shelf's "Trailers/" or "Samples/" holds its films' trailers and samples, named as
	// the films are; a film called so names its folder with its year, "Trailers (2019)/".
	// "Trailers/" at a root's top, or on a shelf of shelves ("Media/Library/"), holds trailers too.
	// Its "Extras/" is walked: a film may be kept there (shelfBonusExtras).
	shelfTrailers := shelfExtras[lower] && parent != "." && amongFilms(parent) ||
		shelfExtras[lower] && (parent == "." || shelfAt(parent) && !underShows(name) && !showsFolderNames[folderKey(path.Base(parent))])
	return skippedDirs[lower] || prefixed || seasonExtrasFolder(name) || filmSpecials || subsDirs[lower] || shelfTrailers ||
		(parent != "." && extrasFolder(name, parent) && titled && !shelfAt(parent)) ||
		(lower == "specials" && parent != "." && isSeasonFolder(parent) && !strings.EqualFold(path.Base(parent), "specials"))
}

// holdsTitle says whether a folder, rel to the root with its entries, is a title's own: a season
// folder, one named as a title is with its year ("Ashgrove (2015)"), or one holding a video, a
// disc or a season folder. An extras folder in it is that title's extras.
func holdsTitle(rel string, entries []fs.DirEntry) bool {
	if rel == "." {
		return false
	}
	if title, year, _ := searchQuery(path.Base(rel), 0); isSeasonFolder(rel) || title != "" && year != 0 {
		return true
	}
	for _, e := range entries {
		lower := strings.ToLower(e.Name())
		switch {
		case e.IsDir() && (discDirs[lower] || isSeasonFolder(path.Join(rel, e.Name()))):
			return true
		case !e.IsDir() && (videoExtensions[filepath.Ext(lower)] && !extraIn(e.Name(), path.Base(rel)) || filepath.Ext(lower) == ".iso"):
			return true
		}
	}
	return false
}

// relTo is path relative to root with forward slashes, "." for the root itself.
func relTo(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
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
	// Sidecars are the file's own NFO, artwork and trickplay files, by path, so a changed sidecar
	// changes the file's identity digest without being read. A trickplay file's Stat is zero: only
	// its being there counts.
	Sidecars map[string]Stat
	// Sole is whether it is its folder's only video, so the folder's own sidecars are its.
	Sole bool
	// Notes are what its folder's videos say of it together: whether they number themselves after
	// a name they share.
	Notes folderNotes
	// Part is which part of a movie it is, from 1, when its folder holds the movie in parts
	// ("cd1", "cd2"), or holds one part of it in a folder of its own ("CD1/"); 0 for a whole file.
	// PartSet names the set it is a part of, the same for every part of one movie.
	Part    int
	PartSet string
	// Problem is why the path is not imported, "" for a media file.
	Problem string
	// Top is which of step 1's units the file is under (a top-level entry, or an entry of one, as
	// walker.units counts them), so the import can tell how many units' files it has written.
	Top int
}

// Walk is one walk of one root.
type Walk struct {
	Files []File
	// Complete is false when any folder or file could not be read, and Unread lists each one: a
	// file missing under one of them proves nothing, but a file missing anywhere else is gone.
	Complete bool
	Unread   []string
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
	at, parent := w.root, "."
	for _, seg := range strings.Split(rel, "/") {
		if _, err := os.Stat(filepath.Join(at, ".ignore")); err == nil {
			return true
		}
		titled := false
		if extrasFolder(seg, parent) {
			entries, _ := os.ReadDir(at)
			titled = holdsTitle(parent, entries)
		}
		if skippedDir(parent, seg, titled) || discDirs[strings.ToLower(seg)] {
			return true
		}
		at, parent = filepath.Join(at, seg), path.Join(parent, seg)
	}
	return false
}

type walker struct {
	root     string
	realRoot string
	// shelf is the root's own name when it is a shelf of a kind or an audience's ("/media/TV",
	// "/media/Movies", "/media/Kids"), "" otherwise: its folders are read as if under it
	// (folderNotes.Root).
	shelf   string
	visited map[string]bool
	// nfo is each folder's NFO and artwork files, kept while the walk is under it.
	nfo map[string]map[string]Stat
	// videos counts each folder's videos, kept alongside nfo.
	videos map[string]int
	// steps counts folders listed and files read.
	steps *atomic.Int64
	// report hears a whole root's walk go, for the import's progress; nil hears nothing.
	report *walkReport
	// top is step 1's unit being walked: a top-level entry, or an entry of one (units).
	top int
	// firstUnit is each top-level folder counted by its own entries, by path: its first unit.
	firstUnit map[string]int
	// listed is a top-level folder's entries, read once by units and taken by dir, so a small
	// root's folders are each listed once.
	listed map[string][]os.DirEntry
	out    Walk
	// skipped are paths whose names are not UTF-8.
	skipped []string
}

func newWalker(root string, steps *atomic.Int64) *walker {
	if steps == nil {
		steps = new(atomic.Int64)
	}
	w := &walker{
		root: filepath.Clean(root), shelf: rootShelf(root), visited: map[string]bool{},
		nfo: map[string]map[string]Stat{}, videos: map[string]int{}, steps: steps, firstUnit: map[string]int{},
		listed: map[string][]os.DirEntry{},
	}
	w.out.Complete = true
	return w
}

// unitsWanted is how many units step 1 counts a root by at least, when it can: a root with fewer
// top-level entries ("/media" holding Movies and TV) is counted by what each of its folders holds.
const unitsWanted = 50

// units is how many units step 1 counts the root by, and the first unit of each top-level entry. A
// root with unitsWanted top-level entries or more counts each as one. One with fewer counts each
// top-level folder by its own entries, listed now, and notes each folder's first unit for its walk;
// a file, or a folder that is empty or can't be listed, is one.
func (w *walker) units(root string, entries []os.DirEntry) (int, []int) {
	bases := make([]int, len(entries))
	if len(entries) >= unitsWanted {
		for i := range entries {
			bases[i] = i
		}
		return len(entries), bases
	}
	n := 0
	for i, e := range entries {
		bases[i] = n
		size := 1
		if e.IsDir() {
			path := filepath.Join(root, e.Name())
			if inside, err := os.ReadDir(path); err == nil && len(inside) > 0 {
				size = len(inside)
				w.firstUnit[path] = n
				w.listed[path] = inside
			}
			w.steps.Add(1)
		}
		n += size
	}
	return n, bases
}

// unread records a folder or file the walk could not read: what lay under it is unknown.
func (w *walker) unread(path string) {
	w.out.Complete = false
	w.out.Unread = append(w.out.Unread, path)
}

func (w *walker) dir(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// A folder reached twice, through a symlink inside the root, is walked once.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		w.unread(dir)
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
	entries, err := w.listed[dir], error(nil)
	if entries == nil {
		entries, err = os.ReadDir(dir)
		w.steps.Add(1)
	}
	delete(w.listed, dir)
	if err != nil {
		w.unread(dir)
		return nil
	}
	nfos := map[string]Stat{}
	var videoNames []string
	for _, e := range entries {
		if e.Name() == ".ignore" {
			return nil
		}
		if e.IsDir() {
			continue
		}
		if lower := strings.ToLower(e.Name()); videoExtensions[filepath.Ext(lower)] && !extraIn(e.Name(), filepath.Base(dir)) {
			videoNames = append(videoNames, e.Name())
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
	folder := ""
	if dir != w.root {
		folder = filepath.Base(dir)
	}
	shows := dir != w.root && underShows(path.Join(w.shelf, w.rel(dir)))
	extras := siblingExtras(folder, videoNames, shows || seasonDirOf(w.rel(dir)) != "")
	for name := range extrasBeside(videoNames) {
		extras[name] = true
	}
	for name := range shelfBonusExtras(w.rel(dir), videoNames) {
		extras[name] = true
	}
	for name := range shelfSamples(path.Join(w.shelf, w.rel(dir)), videoNames) {
		extras[name] = true
	}
	videoNames = slices.DeleteFunc(videoNames, func(name string) bool { return extras[name] })
	if !shows && seasonDirOf(w.rel(dir)) == "" {
		ripped := rippedExtras(folder, videoNames, func(name string) int64 {
			i := slices.IndexFunc(entries, func(e fs.DirEntry) bool { return e.Name() == name })
			if st, ok := w.stat(filepath.Join(dir, name), entries[i]); ok {
				return st.Size
			}
			return 0
		})
		videoNames = slices.DeleteFunc(videoNames, func(name string) bool { return ripped[name] })
		for name := range ripped {
			extras[name] = true
		}
	}
	notes, parts := notesOf(videoNames), partsOf(folder, videoNames)
	if shows || showLabelled(folder) {
		// A shows' folder's "Part 1", "Part 2" are a miniseries' episodes (numberingInFolder), as
		// are a folder's labelled a show's on any shelf, "Movies/The Long Watch Miniseries/".
		parts = nil
	}
	marked := 0
	for _, name := range videoNames {
		if episodeMark.MatchString(name) {
			marked++
		}
	}
	// In a shows' catch-all, one marked episode says a show's folder, as does one that names the
	// folder's show: "Kids/Puffin Patrol/Puffin Patrol 1x02.mp4".
	ownMarked := dir != w.root && slices.ContainsFunc(videoNames, func(name string) bool {
		show := looseSeries(name)
		return show != "" && sameWords(show, showFolderName(folder))
	})
	notes.Episodes = marked >= 2 || marked == 1 && (shows || ownMarked) || slices.ContainsFunc(entries, func(e fs.DirEntry) bool {
		return e.IsDir() && dir != w.root && isSeasonFolder(path.Join(w.rel(dir), e.Name()))
	})
	notes.Leading = leadingEpisodes(folder, videoNames)
	notes.Series = dir != w.root && namedSeries(folder, videoNames, onAudienceShelf(path.Join(w.shelf, w.rel(dir))))
	notes.Titled = dir != w.root && titledEpisodes(path.Join(w.shelf, w.rel(dir)), videoNames)
	notes.Dashed = dashedShows(path.Join(w.shelf, w.rel(dir)), videoNames)
	notes.Root = w.shelf
	notes.Years = looseYears(videoNames)
	notes.MonthFirst = monthFirstDays(videoNames)
	for _, e := range entries {
		if e.IsDir() {
			notes.Folders = append(notes.Folders, e.Name())
		}
	}
	// A season folder's episodes name their show, not the season: it is never a dump.
	notes.Dump = dir != w.root && seasonDirOf(w.rel(dir)) == "" && len(idsIn(filepath.Base(dir))) == 0 &&
		dumpFolder(filepath.Base(dir), videoNames, showsFolderNames[folderKey(path.Base(path.Dir(w.rel(dir))))])
	// A folder holding one video, named by a part mark alone, may be one part of a movie whose
	// parts each have a folder: the movie's folder is the set.
	folderPart := 0
	if len(videoNames) == 1 {
		folderPart = partFolderOf(dir)
	}
	// A movie in parts is one video: its folder's poster is the movie's.
	movies := len(videoNames) - len(parts)
	for _, n := range parts {
		if n == 1 {
			movies++
		}
	}
	w.nfo[dir], w.videos[dir] = nfos, movies
	defer delete(w.nfo, dir)
	defer delete(w.videos, dir)

	top := dir == w.root
	var bases []int
	if top {
		var units int
		units, bases = w.units(dir, entries)
		w.report.counted(units)
	}
	first, inTop := w.firstUnit[dir]
	titled := holdsTitle(w.rel(dir), entries)
	for i, e := range entries {
		// The units before this one are listed.
		switch {
		case top:
			w.report.listed(bases[i])
			w.top = bases[i]
		case inTop:
			w.report.listed(first + i)
			w.top = first + i
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
			// A link the walk can't follow is listed, not passed over in silence: its target is
			// gone, or lies outside the root.
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				w.problem(path, Stat{}, ProblemBrokenLink)
				continue
			}
			if _, ok := canonicalPath(w.root, w.realRoot, target); !ok {
				w.problem(path, Stat{}, ProblemLinkOutside)
				continue
			}
			info, err := os.Stat(target)
			if err != nil {
				w.problem(path, Stat{}, ProblemBrokenLink)
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
		case isDir && skippedDir(w.rel(dir), e.Name(), titled):
		case isDir && bonusDisc(e.Name(), videoNames, shows || seasonDirOf(w.rel(dir)) != ""):
		case isDir && discDirs[lower]:
			w.problem(path, Stat{}, ProblemDiscImage)
		case isDir:
			if err := w.dir(ctx, path); err != nil {
				return err
			}
		case filepath.Ext(lower) == ".iso" || filepath.Ext(lower) == ".img":
			st, _ := w.stat(path, e)
			w.problem(path, st, ProblemDiscImage)
		case videoExtensions[filepath.Ext(lower)] && !extraIn(e.Name(), filepath.Base(dir)) && !extras[e.Name()]:
			st, ok := w.stat(path, e)
			if !ok {
				w.unread(path)
				continue
			}
			part, set := parts[e.Name()], ""
			switch {
			case part > 0:
				inFolder, _, _ := partIn(e.Name())
				set = w.rel(dir) + "\x00" + inFolder
			case folderPart > 0:
				part, set = folderPart, w.rel(filepath.Dir(dir))+"\x00folders"
			}
			w.out.Files = append(w.out.Files, File{
				Path: path, Rel: w.rel(path), Stat: st,
				Sidecars: w.sidecars(dir, e.Name()), Sole: w.videos[dir] <= 1, Notes: notes, Part: part, PartSet: set, Top: w.top,
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

func (w *walker) rel(path string) string { return relTo(w.root, path) }

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
