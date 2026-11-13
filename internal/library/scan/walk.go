package scan

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
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

// extrasFolder says whether a folder named name in parent is an extras folder: named by its kind
// (extrasKinds), "Extras", "Bloopers", "Making Of", or by its title then its kind, "Brine
// Rock/Brine Rock S01 Extras". In a title's own folder one is skipped: only movies and episodes
// become titles (0047). Elsewhere it is a folder of titles: a show named "Shorts" at the top,
// "Movies/Kids/Shorts" or "TV/Cartoons/Clips".
func extrasFolder(name, parent string) bool {
	if extrasName(name) {
		return true
	}
	// A release's material in a title's folder, "[Grp] Kasa Hoshi [BD]/CDs/", "…/Scans/", holds
	// its soundtrack and its booklet's pages, never an episode; at the top or among shelves the
	// name may be a shelf's.
	if packMaterial[normName(name)] && parent != "." && !shelfAt(parent) {
		return true
	}
	// Its kind first, then its disc's label: "Extras - Disc 2", "Bonus Features Disc 3".
	if words := strings.Fields(normName(name)); parent != "." {
		for n := 1; n <= 2 && n < len(words); n++ {
			if partFolder.MatchString(strings.Join(words[len(words)-n:], " ")) && extrasName(strings.Join(words[:len(words)-n], " ")) {
				return true
			}
		}
	}
	head := extrasAfter(name)
	if len(head) == 0 || parent == "." {
		return false
	}
	// A disc's label names its title's extras as the title does: "Disc 2 - Extras".
	if partFolder.MatchString(strings.Join(head, " ")) {
		return true
	}
	// The title is its show's, or its film's without its year: "Sallowmere (2008)/Sallowmere Extras".
	title, _, _ := searchQuery(path.Base(parent), 0)
	for _, own := range [][]string{showWordsOf(path.Base(parent)), strings.Fields(normName(title))} {
		if len(own) > 0 && len(head) >= len(own) && slices.Equal(head[:len(own)], own) {
			return true
		}
	}
	return false
}

// packMaterial are the folders a release's pack keeps its material in beside its episodes, as
// normName writes them.
var packMaterial = map[string]bool{
	"cds": true, "cd": true, "scans": true, "scan": true, "ost": true, "osts": true, "soundtrack": true,
	"soundtracks": true, "booklet": true, "booklets": true, "fonts": true, "covers": true, "artwork": true,
}

// bonusDisc says whether a disc's folder ("Disc 2", "DVD2") beside a film's own video holds the
// film's extras: the folder of a film's parts holds no film of its own (partFolderOf), and a
// show's or a season's discs hold its episodes. videos are the folder's own videos.
func bonusDisc(name string, videos []string, shows bool) bool {
	return !shows && partFolder.MatchString(name) && len(videos) == 1 && !episodeMark.MatchString(videos[0])
}

// seasonExtrasFolder says whether a folder is a season's extras by its own name, wherever it is:
// "Corran.Hill.S04.Extras", "Corran Hill Season 4 Extras", "Kettle.Moor.S01.Bonus.Features",
// "Pike.Mere.S02.Extras.720p".
func seasonExtrasFolder(name string) bool {
	head := extrasAfter(name)
	if len(head) < 2 {
		return false
	}
	return slices.ContainsFunc(head[1:], seasonToken.MatchString) || namedSeasonOf(strings.Join(head, " ")) > 0
}

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

// shelfExtras are the folders a films' shelf keeps its films' trailers and samples in.
var shelfExtras = map[string]bool{"trailers": true, "teasers": true, "samples": true}

// plexSuffix matches Plex's suffix for a film's extra, in lower case after a dash: "-short",
// "-behindthescenes".
var plexSuffix = regexp.MustCompile(`-(?:trailer|short|scene|clip|featurette|behindthescenes|deleted|interview|other|extra)$`)

// shelfBonusExtras is which videos in a films' shelf's bonus folder, "Movies/Extras/", "Movies/Bonus
// Features/", are its films' extras: those that name no year, "Tamber Sound Featurette". One that
// does, "Movies/Extras/Pinfold (2011).mkv", is a film kept there, never skipped unread. dir is the
// folder, relative to the root.
func shelfBonusExtras(dir string, names []string) map[string]bool {
	if dir == "." || path.Dir(dir) == "." || !shelfBonus[normName(path.Base(dir))] || !amongFilms(path.Dir(dir)) {
		return nil
	}
	out := map[string]bool{}
	for _, name := range names {
		if _, year, _ := searchQuery(plainStem(name), 0); year == 0 {
			out[name] = true
		}
	}
	return out
}

// shelfBonus are the folders a films' shelf keeps its films' bonus videos in, by normName.
var shelfBonus = map[string]bool{
	"extras": true, "bonus": true, "bonus features": true, "special features": true, "featurettes": true,
	"behind the scenes": true, "deleted scenes": true, "interviews": true, "bonus material": true,
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
	videoNames = slices.DeleteFunc(videoNames, func(name string) bool { return extras[name] })
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

// extraName reports whether a video file's lower-cased name marks it as no title whatever is beside
// it: a sample, an anime's creditless opening, a copy still being written ("~"), or a hidden file
// such as a Mac's "._" shadow of a real one. A name with a space after its dot (".45 Calibre
// Kings") is a title's. An extra named by its kind ("- Trailer") is one only beside its title:
// siblingExtras.
func extraName(lowerName string) bool {
	base := strings.TrimRight(strings.TrimSuffix(lowerName, filepath.Ext(lowerName)), "0123456789")
	hidden := strings.HasPrefix(lowerName, "._") || strings.HasPrefix(lowerName, ".") && !strings.Contains(lowerName, " ")
	if hidden || strings.HasPrefix(lowerName, "~") || base == "sample" || strings.HasPrefix(base, "sample-") ||
		creditless.MatchString(lowerName) || (leadingTags.MatchString(lowerName) && animeOpening.MatchString(lowerName)) ||
		siteName.MatchString(strings.TrimSuffix(lowerName, filepath.Ext(lowerName))) ||
		promoNames[strings.TrimSuffix(lowerName, filepath.Ext(lowerName))] || strings.HasSuffix(base, ".partial") ||
		otherKind.MatchString(strings.TrimSuffix(lowerName, filepath.Ext(lowerName))) {
		return true
	}
	// Joined with a dash, a release's sample: "Movie (2001)-sample.mkv"; and its proof after its year
	// or tags, "Brigg (2008)-proof.mkv" ("Bullet-proof" is a title's word).
	// Bracketed, it is a release's note: "Gullfen S01E05 [sample]", "Ashgrove (2015) (Sample)".
	if strings.HasSuffix(base, "-sample") || strings.HasSuffix(base, "[sample]") || strings.HasSuffix(base, "(sample)") ||
		releaseProof.MatchString(base) {
		return true
	}
	// An episode's title set apart by a spaced dash after its mark, "Thornbury S01E07 - Sample", is
	// the episode's in any case: a release joins its sample to its name ("….x264-grp.sample").
	if before, ok := strings.CutSuffix(base, " - sample"); ok && episodeMark.MatchString(before) {
		return false
	}
	// Otherwise it follows a release's own words: "Movie (2001) sample", "Kettle Bay S01E02
	// sample", "kettle.bay.s01e02.720p.hdtv.x264-grp.sample"; "The Last Sample", "The.Last.Sample"
	// and "Free Sample" are titles.
	for _, sep := range []string{".", "_", " "} {
		before, ok := strings.CutSuffix(base, sep+"sample")
		if !ok {
			continue
		}
		fields := strings.FieldsFunc(strings.TrimRight(before, " ._-"), func(r rune) bool { return r == ' ' || r == '.' || r == '_' })
		if len(fields) == 0 {
			return false
		}
		last := fields[len(fields)-1]
		if strings.HasSuffix(last, ")") || strings.HasSuffix(last, "]") || episodeMark.MatchString(last) || yearToken.MatchString(last) {
			return true
		}
		// The release's words, and its group's after a dash: "x264-grp".
		return slices.ContainsFunc(strings.Split(last, "-"), releaseToken)
	}
	return false
}

// extraFile is extraName for a file's name as it is written: an episode titled as a household
// titles one, "Thornbury S01E07 - Sample", is that episode, where a release's lower-case "kettle bay
// s01e02 - sample" is its sample.
func extraFile(name string) bool {
	return extraName(strings.ToLower(name)) && !titledSample.MatchString(stem(name))
}

// extraIn is extraFile for a file in its folder, named folder: a video named as its own folder is
// that folder's title whatever word it is, "Movies/Sample/Sample.mkv", "Sample (2021)/Sample.mkv".
func extraIn(name, folder string) bool {
	return extraFile(name) && !namedAsFolder(name, folder)
}

// namedAsFolder says whether a video is named as its folder is, with or without the folder's year
// and notes: "Sample/Sample.mkv", "Featurette (1999)/Featurette.mkv". folder is "" for a root's top.
func namedAsFolder(name, folder string) bool {
	if folder == "" {
		return false
	}
	if strings.EqualFold(stem(name), folder) {
		return true
	}
	// Only its year and its release's notes may go: "Ashgrove (2015)-sample" is no "Ashgrove".
	own := strings.TrimSpace(bracketYear.ReplaceAllString(trimReleaseNotes(plainStem(name)), ""))
	title, _, _ := searchQuery(folder, 0)
	return title != "" && sameWords(own, title)
}

// titledSample matches an episode's mark, then its title "Sample", capitalised, after a dash set
// apart: "S01E07 - Sample", "S01E07.-.Sample".
var titledSample = regexp.MustCompile(`(?i:s\d{1,4}[ ._]?e\d{1,4})[ ._]-[ ._]Sample$`)

// siteName matches a video named by nothing but a site, the promo a tracker or a release group
// leaves in a download: "RARBG.com.mp4", "www.Torrent.to.mkv". A suffix that is also a word needs
// the "www.": "Trust.Me.mkv" and "Nine.To.mkv" are films.
var siteName = regexp.MustCompile(`^(?:www\.[a-z0-9-]+(?:\.[a-z0-9-]+)*\.[a-z]{2,6}|[a-z0-9-]+\.(?:com|org|net|info|xyz|biz|club))$`)

// otherKind matches a name that ends in another kind of file's extension before a video's, as a
// site's junk does to pass for one: "RARBG.txt.mp4", "Read Me.url.mkv", "setup.exe.avi".
var otherKind = regexp.MustCompile(`\.(?:txt|nfo|url|lnk|exe|scr|bat|com|htm|html|pdf|jpg|jpeg|png|gif|torrent|zip|rar)$`)

// releaseProof matches a release's proof clip, joined after its year, tags or group: "brigg
// (2008)-proof", "brigg.2008.1080p.bluray.x264-grp-proof".
var releaseProof = regexp.MustCompile(`(?:[)\]]|\d{4}|x26[45]|-[a-z0-9]+)-proof$`)

// promoNames are the names a release group's or tracker's promo clip goes by, in lower case, with
// no site's ending: "ETRG.mp4", "YTS.mp4". A download still being written ends ".partial".
var promoNames = map[string]bool{
	"etrg": true, "rarbg": true, "yts": true, "yify": true, "torrentgalaxy": true, "tgx": true,
	"extratorrent": true, "1337x": true, "rarbg_do_not_mirror": true, "torrent downloaded from": true,
}

// extraKinds are the words an extra is named by, as Jellyfin, Plex and households name them; a
// longer one is tried before a word it starts with.
var extraKinds = [][]string{
	{"behind", "the", "scenes"}, {"making", "of"}, {"gag", "reel"}, {"music", "video"}, {"alternate", "ending"},
	{"short", "film"}, {"deleted", "scenes"}, {"deleted", "scene"}, {"official", "trailer"},
	{"teaser", "trailer"}, {"theatrical", "trailer"}, {"directors", "commentary"}, {"director", "commentary"},
	{"audio", "commentary"}, {"blooper", "reel"}, {"tv", "spot"}, {"photo", "gallery"},
	{"promo"}, {"storyboards"}, {"storyboard"}, {"gallery"}, {"sample"}, {"sneak", "peek"},
	{"extended", "scene"}, {"extended", "scenes"}, {"easter", "eggs"}, {"easter", "egg"},
	{"opening", "credits"}, {"end", "credits"}, {"closing", "credits"}, {"q&a"}, {"credits"}, {"intro"},
	{"recap"}, {"bonus"}, {"preview"}, {"trailers"}, {"teasers"}, {"featurettes"}, {"interviews"},
	{"promos"}, {"previews"}, {"recaps"}, {"clips"}, {"extras"},
	{"trailer"}, {"teaser"}, {"featurette"}, {"interview"}, {"behindthescenes"}, {"deletedscene"},
	{"deletedscenes"}, {"deleted"}, {"bloopers"}, {"outtakes"}, {"commentary"}, {"scene"}, {"short"},
	{"clip"}, {"extra"}, {"other"}, {"theme"}, {"creditless"}, {"textless"}, {"ncop"}, {"nced"},
}

// extraKindAt is where in a name's words its extra's kind starts, -1 for none. A number joined on
// is its count: "trailer2".
func extraKindAt(words []string) int {
	bare := make([]string, len(words))
	for i, w := range words {
		bare[i] = strings.NewReplacer("'", "", "’", "").Replace(strings.TrimRight(w, "0123456789"))
	}
	// The kind follows the title, which may hold a kind's word of its own: "Ferry Extra (1982)
	// 1080p-trailer" is a trailer.
	last := -1
	for i := 0; i < len(bare); i++ {
		for _, kind := range extraKinds {
			if len(bare)-i >= len(kind) && slices.Equal(bare[i:i+len(kind)], kind) {
				last = i
				i += len(kind) - 1
				break
			}
		}
	}
	return last
}

// kindEnds says whether a name's words from its extra's kind are that kind alone, with a count or a
// release's words after it: "Trailer 2", "Deleted Scenes", "trailer 1080p"; "Scene of the Crime"
// and "Clip Show" are titles. A kind first, "Featurette Costumes", is a household's extra named by
// what it shows, read beside its title (siblingExtras).
func kindEnds(words []string) bool {
	for _, kind := range extraKinds {
		bare := func(w string) string { return strings.TrimRight(w, "0123456789") }
		if len(words) < len(kind) || !slices.EqualFunc(words[:len(kind)], kind, func(w, k string) bool { return bare(w) == k }) {
			continue
		}
		// "Making of Season 1" is its season's extra.
		if _, season := seasonIn(strings.Join(words[len(kind):], " ")); season && len(words) > len(kind) {
			return true
		}
		return !slices.ContainsFunc(words[len(kind):], func(w string) bool { return !digits(w) && !releaseToken(w) })
	}
	return false
}

// extrasBeside is which of a folder's videos are extras of the episodes or the season beside them,
// read whole by the folder:
//   - An episode's name titled by a kind of extra alone after its mark, "S01E02 - Sample",
//     "S01E01 - Commentary", is that episode's extra when another video beside it numbers the same
//     episode. Alone, it is the episode, titled so.
//   - A show's season and a kind of extra with no episode's mark, "Fernhollow.S01.Bloopers",
//     "Gullhaven - Season 1 Promo", is the season's extra, as a folder so named is
//     (seasonExtrasFolder).
func extrasBeside(names []string) map[string]bool {
	var out map[string]bool
	add := func(name string) {
		if out == nil {
			out = map[string]bool{}
		}
		out[name] = true
	}
	titledExtra := func(name string) bool {
		_, after, ok := strings.Cut(stem(name), " - ")
		for ok && episodeMark.MatchString(after) {
			_, after, ok = strings.Cut(after, " - ")
		}
		words := extraWords(after)
		return ok && len(words) > 0 && extraKindAt(words) == 0 && kindEnds(words)
	}
	for _, name := range names {
		mark, marked := numberingIn(name)
		if !marked {
			if seasonExtrasFolder(stem(name)) || seasonKindFirst(name) || dashedExtra(name) {
				add(name)
			}
			continue
		}
		if !titledExtra(name) {
			continue
		}
		if slices.ContainsFunc(names, func(other string) bool {
			n, ok := numberingIn(other)
			return other != name && ok && !titledExtra(other) && n.Season == mark.Season && n.Episode == mark.Episode
		}) {
			add(name)
		}
	}
	return out
}

// seasonKindFirst says whether a video is named by its show, its season with no episode, then a
// kind of extra and what it shows: "Saltcove.S02.Bonus.Cast.Q&A", "Gullhaven S1 Featurette
// Costumes". A season's mark and a title, "Saltcove S02 The Long Night", is no extra, nor is one
// whose title starts with an everyday word a kind is also: "Saltcove S02 Short Stories".
func seasonKindFirst(name string) bool {
	words := extraWords(name)
	for i := 1; i < len(words)-1; i++ {
		if !seasonToken.MatchString(words[i]) {
			continue
		}
		rest := words[i+1:]
		return !everydayKinds[rest[0]] && slices.ContainsFunc(extraKinds, func(kind []string) bool {
			return len(rest) >= len(kind) && slices.Equal(rest[:len(kind)], kind)
		})
	}
	return false
}

// dashedExtra says whether a video with no episode's mark is named by its title, a dash, then a
// kind of extra: first, of the kinds no title starts with, "Ashlow Run - Featurette - Sets", "Kame
// Tsuki - Creditless ED", or after a word that says whose it is, "Ashlow Run - Cast Interview",
// "[Grp] Kame Tsuki - Next Episode Preview". "Ashlow Run - The Interview" and "Hilltop - Interview
// with a Ghost" may be episodes' titles, and stay them; so does a recap, its show's season 0's.
func dashedExtra(name string) bool {
	head, after, ok := strings.Cut(stem(leadingTags.ReplaceAllString(name, "")), " - ")
	if !ok || strings.TrimSpace(head) == "" {
		return false
	}
	words := extraWords(trimReleaseNotes(after))
	if len(words) == 0 {
		return false
	}
	if extraFirst[words[0]] {
		return true
	}
	for i := 1; i < len(words); i++ {
		if extraOwners[words[i-1]] && kindEnds(words[i:]) {
			return true
		}
	}
	return false
}

// extraFirst are the kinds of extra no episode's title starts with.
var extraFirst = map[string]bool{
	"featurette": true, "featurettes": true, "creditless": true, "textless": true, "ncop": true, "nced": true,
	"bloopers": true, "outtakes": true, "storyboards": true, "behindthescenes": true, "deletedscenes": true,
}

// extraOwners are words before a kind of extra that say whose it is: "Cast Interview", "Next
// Episode Preview", "Creditless Opening".
var extraOwners = map[string]bool{
	"cast": true, "crew": true, "director": true, "directors": true, "creator": true, "episode": true,
	"season": true, "series": true, "creditless": true, "textless": true, "extended": true, "bonus": true,
}

// everydayKinds are kinds of extra that are everyday words too, and so start titles.
var everydayKinds = map[string]bool{
	"short": true, "scene": true, "clip": true, "other": true, "theme": true, "extra": true, "intro": true,
	"credits": true, "end": true, "opening": true, "closing": true, "music": true, "easter": true,
}

// episodeExtra matches Jellyfin's mark for an episode's own extra, joined to its name with a dash:
// "Kettle Bay - S01E02-trailer", "… - Tidewater-deleted". " - Trailer", spaced, is the episode's
// title. So is one joined with a dot straight after the episode's mark, "Coldharbour
// S01E02.Trailer": a title follows its mark apart, " - Trailer".
var episodeExtra = regexp.MustCompile(`(?i)(?:-|[ ._]s\d{1,2}e\d{1,3}\.)(?:trailer|featurette|behindthescenes|deleted|deletedscene|interview|scene|short|clip|extra)\d*$`)

// titleWordExtra matches an episode's title that ends in an everyday word joined on with a dash,
// as a hyphenated title does: "Behind-the-Scene", "Cut-Short", "Paper-Clip", "Re-Interview". A
// mark joined to the episode's number ("S01E02-short") is Jellyfin's, as is one it writes in lower
// case after a title ("Tidewater-clip").
var titleWordExtra = regexp.MustCompile(`[a-zA-Z]-(?:(?i:short|scene)|[A-Z][a-z]+)$`)

// extraWords are a name's lower-cased words, split at its separators and brackets; an apostrophe
// stays in its word ("barney's"), as the title it is matched against keeps it.
func extraWords(name string) []string {
	return strings.FieldsFunc(strings.ToLower(stem(name)), func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("._-()[]{}#,", r)
	})
}

// siblingExtras is which of a folder's videos are extras of a title beside them: named by their
// title then their kind ("Ashgrove (2015) - Official Trailer", "Ashgrove.2015.Trailer.1080p",
// "Ashgrove (2015) [Trailer]"), where the folder holds that title's own video or is named as it
// is with its year; or named by their kind alone ("Trailer 2", "Gag Reel", "theme") in a title's
// own folder. A name with no title beside it is a title of its own, kind words and all
// ("Night Trailer", "Cut-Short", "The Other Side (2019)"), as is one with its own year after its
// kind ("Ashgrove Short (2017)"), and an episode, whose mark says what it is ("S02E08 - Trailer").
// folder is the folder's name, "" for a root's top.
func siblingExtras(folder string, names []string, shows bool) map[string]bool {
	// A title is keyed by its letters alone: "E.T.A. the Ferry" and its extra's words, "e t a the
	// ferry", are one.
	compact := func(title string) string { return strings.ReplaceAll(normName(title), " ", "") }
	key := func(words []string) string {
		title, _, _ := searchQuery(strings.Join(words, " "), 0)
		return compact(title)
	}
	titles := map[string]bool{}
	titled := false
	if title, year, _ := searchQuery(folder, 0); folder != "" && title != "" && year != 0 {
		titles[compact(title)], titled = true, true
	}
	type marked struct {
		name  string
		words []string
		at    int
	}
	var kinds []marked
	out := map[string]bool{}
	episodes := slices.ContainsFunc(names, episodeMark.MatchString)
	// The film a film's own folder holds, named as the folder is: "Corbel Ridge (2013).mkv". One
	// ending in a kind of extra, "Corbel Ridge (2013) 1080p-trailer", is no film; "Sample Lane
	// (2011)" is.
	film := titled && slices.ContainsFunc(names, func(name string) bool {
		words := extraWords(name)
		at := extraKindAt(words)
		return !strings.Contains(stem(name), " - ") && titles[key(strings.Fields(stem(name)))] && (at <= 0 || !kindEnds(words[at:]))
	})
	// A video named as a household names a film, not as MakeMKV numbers what it rips, is the film:
	// the disc's other titles left beside it, "Gilded Road_t03", are the disc's extras.
	renamed := !shows && !episodes && slices.ContainsFunc(names, func(name string) bool {
		return !makeMKVTitle.MatchString(plainStem(name))
	})
	for _, name := range names {
		words := extraWords(name)
		at := extraKindAt(words)
		if renamed && makeMKVTitle.MatchString(plainStem(name)) {
			out[name] = true
			continue
		}
		// In a release's pack, its group's tag on the folder, "[Grp] Tora Michi [01-24]/Tora Michi -
		// Menu.mkv" is the release's extra as a tagged name's is (extraName).
		if leadingTags.MatchString(folder) && animeOpening.MatchString(stem(name)) {
			out[name] = true
			continue
		}
		// A hidden copy of a video beside it, ".Fenwick Garth (2003).mkv", is a shadow a copy or a
		// sync left, never a second copy; ".45 Calibre Kings" alone is a title.
		if strings.HasPrefix(name, ".") && slices.ContainsFunc(names, func(other string) bool {
			return other != name && strings.EqualFold(plainStem(other), plainStem(name[1:]))
		}) {
			out[name] = true
			continue
		}
		// Beside that film, Plex's own suffix, joined on in lower case, says its extra whatever the
		// name before it: "Ashwick Short Film-short", "… Gala-scene". "Cut-Short" is a title, and so
		// is a name all in lower case, "cut-short", which can't say which it is.
		if film && plexSuffix.MatchString(stem(name)) && name != strings.ToLower(name) {
			out[name] = true
			continue
		}
		// Beside that film, "<film> - <anything else>" is its extra, whatever the household called
		// it ("- Animatic", "- Press Conference"), unless what follows says which copy of the film
		// it is: "- 1080p", "- Directors Cut", "- Part 2".
		if before, after, ok := strings.Cut(stem(name), " - "); film && ok && titles[key(strings.Fields(before))] && !filmVersion(name, after) &&
			!episodeMark.MatchString(name) && airDateIn(stem(name)) == "" {
			out[name] = true
			continue
		}
		// In a show's folder, a name its kind ends that only an extra is named by, with no episode's
		// mark, is the show's extra: "Gannet Rock - Official Trailer", "Gannet Rock S02 Trailer",
		// "trailer.mkv". "Gannet Rock S01E05 - The Trailer" is an episode.
		if shows && at >= 0 && strongExtraKind(words[at:]) && kindEnds(words[at:]) && !episodeMark.MatchString(name) {
			out[name] = true
			continue
		}
		// A name written all in lower case can't say "Paper-Clip" from Jellyfin's "Tidewater-clip":
		// it is the episode, never skipped unread.
		if episodeMark.MatchString(name) && episodeExtra.MatchString(stem(name)) && !titleWordExtra.MatchString(stem(name)) &&
			name != strings.ToLower(name) {
			out[name] = true
			continue
		}
		// A kind's word before the title's year is the title's: "Ferry Extra (1982) 1080p". Among a
		// show's files, a kind's word with more after it is a title's, "Hilltop - Scene of the Crime",
		// and so is a dated name's, "Quillon Hall - 2018-04-02 - Recap": a day says an episode as its
		// mark does. In a film's own folder, "… - Interview with Cast" is its extra.
		// Beside a show's marked episodes, a word an episode is as often titled by, "Hilltop -
		// Credits", "… - Recap", is its title: only a kind only an extra is named by is an extra.
		if at < 0 || episodeMark.MatchString(name) || slices.ContainsFunc(words[at:], releaseYear) ||
			at > 0 && !kindEnds(words[at:]) && (!titled || episodes) || airDateIn(stem(name)) != "" ||
			episodes && at > 0 && slices.Contains(titleKinds, words[at]) {
			titles[key(words)] = true
			// An episode's show is a title its extras name: "Cold Coast - Making Of" beside
			// "Cold Coast S01E01".
			if show := looseSeries(name); show != "" {
				title, _, _ := searchQuery(show, 0)
				titles[compact(title)] = true
			}
			continue
		}
		kinds = append(kinds, marked{name, words, at})
	}
	for _, k := range kinds {
		if k.at == 0 {
			// Its kind alone, and what a household adds: "Trailer 2", "Featurette Costumes", in a
			// title's own folder: one named with its year, or holding that one title. In a catch-all
			// ("Movies/Outtakes.mkv") it is a title of its own, and so is one named as its own
			// folder's title: "Featurette (1999)/Featurette.mkv" is the film.
			if (titled || len(titles) == 1 && !catchAllFolder(folder)) && !namedAsFolder(k.name, folder) {
				out[k.name] = true
			}
			continue
		}
		// A title's year, then a kind only an extra is named by, says an extra with no title beside
		// it: "Movies/Gravel Peak (2003) Trailer.mp4".
		// In a film's own folder, one named with its year, a trailer is an extra whichever film it
		// names, "Glimmerfold Sequel Teaser" or another's, "Ironvale Trailer"; one named as the
		// folder's film is the film: "Night Trailer (2010)/Night Trailer.mkv".
		if titles[key(k.words[:k.at])] || slices.ContainsFunc(k.words[:k.at], releaseYear) && strongExtraKind(k.words[k.at:]) ||
			titled && strongExtraKind(k.words[k.at:]) && !titles[key(k.words)] {
			out[k.name] = true
		}
	}
	return out
}

// bonusBeside says whether a video in a film's own dated folder, beside other videos, is named as
// something other than that film: "Gale Spit (2004)/Cast Q and A.mkv", "…/Return to Gale Spit.mkv".
// Its name has no year of its own, is not the film's, does not run on from its title as a sequel's
// does ("Brook (2010)/Brook 2.mkv"), is not the film's name cut short ("asg-1080p", "movie.mkv"),
// and has no episode's or day's mark; a set's part is named before this is asked. It keeps its own name (fallbackName): an unlabelled
// bonus feature waits in Needs you, and a film kept there is its own, never skipped.
func bonusBeside(folder, name string) bool {
	plain := plainStem(name)
	title, _, _ := searchQuery(folder, 0)
	own, _, _ := searchQuery(name, 0)
	if title == "" || own == "" || yearAnywhere.MatchString(plain) || markIn(name) != nil || airDateIn(plain) != "" ||
		makeMKVTitle.MatchString(plain) || genericStem.MatchString(plain) {
		return false
	}
	if sameWords(own, title) || runsOn(own, title) || abbreviates(own, title) || strings.EqualFold(own, initials(title)) {
		return false
	}
	return true
}

// filmVersion says whether what follows a film's name and a dash, after, says which copy of the
// film name is: its quality ("1080p", "4K HDR"), an edition ("Directors Cut") or a part ("Part 2",
// "cd1"), as Plex and Radarr name a film's versions.
func filmVersion(name, after string) bool {
	if editionIn(name) != "" {
		return true
	}
	if _, part, ok := partIn(name); ok && part > 0 {
		return true
	}
	// Radarr's "- [Remux-1080p][DTS-HD MA 5.1][h264]-HiFi": notes in brackets and the group that
	// released it say which copy it is, whatever their words.
	if strings.TrimSpace(releaseGroupTail.ReplaceAllString(bracketedNotes.ReplaceAllString(after, ""), "")) == "" &&
		strings.Contains(after, "[") {
		return true
	}
	words := strings.FieldsFunc(after, func(r rune) bool { return strings.ContainsRune(" ._-[]()", r) })
	if len(words) == 0 {
		return false
	}
	rest := slices.DeleteFunc(slices.Clone(words), releaseToken)
	unmarked := slices.DeleteFunc(slices.Clone(words), releaseMark.MatchString)
	// "- Final Cut", "- Open Matte", "- Black and White", "- English Dub", "- Workprint": words
	// that say how a copy was cut, framed, coloured or voiced say which copy it is.
	versioned := len(rest) > 0 && !slices.ContainsFunc(rest, func(w string) bool { return !versionWords[strings.ToLower(w)] })
	return len(rest) == 0 || editionOnly(rest) || editionOnly(unmarked) || versioned
}

// bracketedNotes matches a release's notes in square brackets anywhere in a name: "[Remux-1080p]".
var bracketedNotes = regexp.MustCompile(`\[[^\]]*\]`)

// releaseGroupTail matches the group a release names last, after a dash: "-HiFi".
var releaseGroupTail = regexp.MustCompile(`^[ ._]*-?[ ._]*[A-Za-z0-9]+$|^[ ._-]*$`)

// versionWords are the words a household names a film's copy by: its cut, framing, colour, voice
// or release. A label of these alone is a version, never an extra; any other word ("Animatic",
// "Press Conference") makes it the film's extra.
var versionWords = map[string]bool{
	"cut": true, "edit": true, "version": true, "final": true, "alternate": true, "alt": true, "rough": true,
	"workprint": true, "open": true, "matte": true, "black": true, "and": true, "white": true, "b&w": true,
	"colorized": true, "colourized": true, "color": true, "colour": true, "theatrical": true, "original": true,
	"home": true, "release": true, "uncut": true, "restored": true, "remaster": true, "remastered": true,
	"redux": true, "censored": true, "uncensored": true, "broadcast": true, "tv": true, "cinema": true,
	"director's": true, "directors": true, "extended": true, "special": true, "edition": true, "ultimate": true,
	"dub": true, "dubbed": true, "sub": true, "subbed": true, "english": true, "language": true,
	"french": true, "german": true, "spanish": true, "italian": true, "japanese": true, "hindi": true,
	"tamil": true, "telugu": true, "korean": true, "mandarin": true, "cantonese": true, "dutch": true,
	"widescreen": true, "fullscreen": true, "pan": true, "scan": true, "3d": true, "imax": true, "hdr": true,
	"sdr": true, "dv": true, "dolby": true, "vision": true, "1st": true, "2nd": true, "us": true, "uk": true,
	"international": true, "domestic": true, "television": true, "longer": true, "hybrid": true, "fan": true,
	"rus": true, "eng": true, "ukr": true, "ger": true, "fre": true, "spa": true, "ita": true, "jpn": true,
	"kor": true, "multi": true,
}

// titleKinds are the kinds an episode is as often titled by: "Credits", "Recap", "Intro".
var titleKinds = []string{"credits", "intro", "recap", "recaps", "preview", "previews", "bonus", "extra", "extras", "other", "theme", "clip", "clips", "scene", "short"}

// strongExtraKinds are the kinds only an extra is named by; "Short", "Clip" or "Theme" after a
// year may be a title's or an edition's.
var strongExtraKinds = []string{
	"trailer", "teaser", "featurette", "behind", "making", "deleted", "bloopers", "outtakes", "gag", "official",
	"theatrical", "behindthescenes", "deletedscene", "deletedscenes", "trailers", "teasers", "featurettes",
}

// strongExtraKind says whether the words an extra's kind starts are a kind only an extra is
// named by.
func strongExtraKind(words []string) bool {
	return len(words) > 0 && slices.Contains(strongExtraKinds, strings.TrimRight(words[0], "0123456789"))
}

var (
	// creditless matches an anime's creditless opening or ending, "NCOP1", "NCED2v2": an extra.
	creditless = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])nc(?:op|ed)\d*(?:v\d+)?(?:[^a-z0-9]|$)`)
	// animeOpening matches a release's extra in its episode's place, " - OP", " - ED2", " -
	// Creditless OP", " - Clean ED", " - Menu 01", " - Preview 03", " - Picture Drama 01", " -
	// Trailer", " - Ending Theme", " - Opening Movie 2": an extra, when a release group's tag says
	// the name is a release's.
	animeOpening = regexp.MustCompile(`(?i)[ ._]-[ ._]+(?:(?:creditless|clean|textless|non[ ._-]?credit)[ ._]+)?(?:op|ed|opening|ending|pv|cm|menu|preview|trailer|teaser|spot|picture[ ._]+drama)(?:[ ._]+(?:theme|song|movie|animation))?(?:[ ._]*\d+)?(?:v\d+)?(?:[ ._]*[\[(.]|$)`)
)
