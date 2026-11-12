package scan

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/localart"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/media/header"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Removal guard (decision 33): a completed walk that would remove more than this share of a root's
// files, or more than this many, is held for the maintainer to confirm.
const (
	holdShare   = 0.10
	holdAtLeast = 500
)

// headerConcurrency is how many header reads run at once (decision 11). The NAS's disk seek, not
// the reader, is the limit: more readers do not read faster.
const headerConcurrency = 4

// arrRetry is how long a folder pass goes on with an arr's failed listing before asking it again.
// A change notification arrives every few seconds on a busy share; asking a down arr again on each
// would wait on it every time.
const arrRetry = time.Minute

// ImportStore is the store the importer reads roots from and keeps its file table in.
type ImportStore interface {
	LibraryRoots(ctx context.Context) (librarystore.LibraryRoots, error)
	CheckLibraryRoot(ctx context.Context, check librarystore.LibraryRootCheck, at time.Time) (librarystore.LibraryRootFlip, error)
	librarystore.LibraryFileStore
}

// Artwork puts a title's artwork in the image service: its poster (#2151 step 10), its fanart as
// the backdrop and its clear logo (#2567). kind is one of ImportedArt; ref is what precedence
// resolved for it, an image file beside the media or a provider's https URL, local first; "" is
// none. The title owns one image per kind: the one it had before is let go.
type Artwork interface {
	Art(ctx context.Context, itemID, kind, ref string) error
}

// ImportedArt is the artwork kinds a title's images are imported for. The rest (banner,
// landscape, clearart, discart) stay references: nothing shows them yet.
var ImportedArt = []string{mp.KindPoster, mp.KindFanart, mp.KindClearlogo}

// Importer is the Library's scanner (#2151 step 5). Each pass walks every in-use root, writes each
// new or changed media file to the inventory as an origin of its root, then reads the headers of
// files that have none yet. The Library's tables are projected from those origins.
type Importer struct {
	Store     ImportStore
	Inventory inventory.Service
	Disk      Disk
	// Managers are the configured arrs, asked on each pass.
	Managers func() []Manager
	// Discover asks every source for its folders again, as Settings → Library's "Check again"
	// does. A full pass calls it first when a connected arr has none of its folders recorded: an
	// arr connected after the folders were set up, whose files would otherwise never meet its
	// records. Nil never asks.
	Discover func(ctx context.Context) error
	// NFO reads sidecars; nil reads none.
	NFO mp.Provider
	// Art lists artwork sidecars; nil lists none. A file's come from the walk's own listing; a
	// series folder's are listed when the series is written.
	Art *localart.Provider
	// Artwork imports the artwork of each title the pass writes; nil imports none.
	Artwork Artwork
	// Catalog is the cache of catalog records a title's details come from; nil reads none.
	Catalog CatalogStore
	// Keys is what the catalog provider fetches; a title it fetches nothing for has no catalog row.
	Keys CatalogKeys
	// Answers is the maintainer's Needs you answers; nil reads none.
	Answers AnswerStore
	// Resolver applies precedence.
	Resolver *precedence.Resolver
	// Header reads one file's container header.
	Header func(ctx context.Context, path string) (header.Header, error)
	// Busy reports whether someone is watching a channel; header reads wait while they are.
	Busy func() bool
	// Enabled is the kill switch, library.scanner.enabled.
	Enabled func() bool
	// Wanted are ids a channel or proposal already asked for; their files' headers are read first.
	Wanted func(ctx context.Context) []inventory.ExternalID
	Now    func() time.Time
	Log    *slog.Logger
	// Poll is the wait between busy checks; zero means 5 s.
	Poll time.Duration
	// HeaderStall is how long one header read may take before the stage gives up on it; zero means
	// headerStall.
	HeaderStall time.Duration
	// WalkStall is how long a walk may make no progress before the pass gives up on it; zero
	// means walkStall.
	WalkStall time.Duration
	// RootReadability hears when a pass finds a root stopped opening, or opens again (decision
	// 32). Nil hears nothing.
	RootReadability func(ctx context.Context, root string, readable bool)
	// RootsChanged hears when a pass finds a root's state changed: it stopped opening or opens
	// again, or it emptied or filled. A screen showing the folders reads them again. Nil hears
	// nothing.
	RootsChanged func()
	// Tracker records the walk's and the header reads' progress, steps 1 and 2 of an import; nil
	// records none.
	Tracker *Tracker
	// TitlesChanged hears each write that changes the Library's titles: an item written, a file's
	// header read, files marked gone. It must not block; nil hears nothing.
	TitlesChanged func()

	// readerVersion stands in for header.Version when not zero: tests read as an older reader.
	readerVersion int
	// nfoRoots are the NFOs read for whether they say their file is an episode.
	nfoRoots nfoRoots

	// run serializes every write to the Library: a pass's walk and import, and each header read's
	// write. The header stage waits for playback without it, so a pass is never held up by a viewer
	// (decision 11).
	run sync.Mutex
	// gen numbers each root a pass imports, so a header read queued by an older import can tell a
	// newer one has written its file since. Guarded by run.
	gen int64
	// index is the arr index the last listing built, reused by folder passes, and indexPaths the
	// path map and arr addresses it was built with. Guarded by run.
	index      *managerIndex
	indexPaths string
	// indexedAt is when the index was last listed; an incomplete one is listed again by a folder
	// pass only after arrRetry.
	indexedAt time.Time

	mu       sync.Mutex
	progress Progress
	// reading is true while a header stage takes files; handoff are the files later passes queued
	// for it, and staged every file the stage has taken, so a pass's hand-off of one it already
	// holds is dropped as it arrives. touched is the import that last wrote each file, one entry a
	// path: a read queued by an older one is dropped. Guarded by mu.
	reading bool
	handoff []pending
	staged  map[string]bool
	touched map[string]int64
	// arrChanged are arr items to read again before the next folder pass uses the index; relist
	// asks for every arr to be listed again. Guarded by mu.
	arrChanged []ArrItem
	relist     bool
	// stalledWalks are roots with a walk or a header read that was given up on, until each ends.
	// Guarded by mu.
	stalledWalks map[string]<-chan struct{}
	// confirmed are roots whose held removal the maintainer confirmed, applied by their next pass.
	confirmed map[string]bool
}

// RootState is how one root's last pass ended.
type RootState string

const (
	RootScanned    RootState = "scanned"
	RootUnreadable RootState = "unreadable"
	RootIncomplete RootState = "incomplete"
	RootHeld       RootState = "held"
	// RootNested is a root inside another in-use root, walked as part of that one.
	RootNested RootState = "nested"
)

// RootResult is one root's last pass.
type RootResult struct {
	Root  string
	State RootState
	// Empty is a root that opened with nothing in it: usually a share that isn't mounted.
	Empty bool
	// Held is how many files the walk no longer found while they wait for confirmation.
	Held int
	// Unreadable are the files the walk could not keep because their names are not UTF-8, relative
	// to the root, each as near its name as text allows: for the maintainer to rename.
	Unreadable []string
}

// Progress is how the scanner's last passes ended, for the status endpoint. How far a running
// import is lives in the Tracker.
type Progress struct {
	CompletedAt time.Time
	Roots       []RootResult
	// ArrIncomplete is true when an arr failed to list on the last pass.
	ArrIncomplete bool
}

// Progress returns how the scanner's last passes ended.
func (im *Importer) Progress() Progress {
	im.mu.Lock()
	defer im.mu.Unlock()
	p := im.progress
	p.Roots = append([]RootResult(nil), p.Roots...)
	return p
}

// Settled reports whether a full pass has completed and neither the walk nor the header reads
// run now.
func (im *Importer) Settled() bool {
	return !im.Progress().CompletedAt.IsZero() && !im.Tracker.Active(StepWalking) && !im.Tracker.Active(StepReading)
}

// titlesChanged tells TitlesChanged.
func (im *Importer) titlesChanged() {
	if im.TitlesChanged != nil {
		im.TitlesChanged()
	}
}

func (im *Importer) setProgress(update func(*Progress)) {
	im.mu.Lock()
	defer im.mu.Unlock()
	update(&im.progress)
}

// pending is a file waiting for its header. Its item is built when the header is read, so a file
// that waits pass after pass has its sidecar read once, by the read that writes it.
type pending struct {
	row  librarystore.LibraryFile
	file File
	id   identity
	b    *builder
	// gen is the import that queued it.
	gen int64
}

// episodeRow is an episode's row in a pass's save list, and its series folder.
type episodeRow struct {
	at        int
	seriesDir string
}

// Run makes one pass. Imports never overlap: a second caller waits for the first. Header reads
// follow the import, in the one header stage (readHeaders).
func (im *Importer) Run(ctx context.Context) error {
	if im.Enabled != nil && !im.Enabled() {
		return nil
	}
	queue, err := im.importAll(ctx)
	if err != nil {
		return err
	}
	if err := im.readHeaders(ctx, queue); err != nil {
		return err
	}
	im.setProgress(func(p *Progress) { p.CompletedAt = im.now() })
	return nil
}

// importAll walks and imports every in-use root, and returns the files waiting for their header.
func (im *Importer) importAll(ctx context.Context) ([]pending, error) {
	im.run.Lock()
	defer im.run.Unlock()
	im.setProgress(func(p *Progress) { p.Roots, p.ArrIncomplete = nil, false })
	// Its total is known once the roots are listed below.
	im.Tracker.Begin(StepWalking, 0)
	defer im.Tracker.End(StepWalking)

	all, err := im.Store.LibraryRoots(ctx)
	if err != nil {
		return nil, err
	}
	if im.Discover != nil && im.arrWithoutFolders(all.Paths) {
		if err := im.Discover(ctx); err != nil {
			im.log().Warn("library scan: could not ask for a newly connected arr's folders", "err", err)
		} else if all, err = im.Store.LibraryRoots(ctx); err != nil {
			return nil, err
		}
	}
	idx := im.arrIndex(ctx, all.Paths, true)
	im.setProgress(func(p *Progress) { p.ArrIncomplete = !idx.complete })
	roots := scannedRoots(all.Roots)
	if err := im.forgetReleased(ctx, roots); err != nil {
		return nil, err
	}

	// The walk's total is the top-level entries of every root it walks, listed once up front.
	reports := map[string]*walkReport{}
	for _, root := range all.Roots {
		if root.InUse && roots[root.Path] {
			r := &walkReport{tracker: im.Tracker, name: filepath.Base(root.Path)}
			if !im.stalled(root.Path) {
				r.entries = topEntries(ctx, root.Path)
			}
			reports[root.Path] = r
			im.Tracker.Grow(StepWalking, r.entries)
		}
	}

	var queue []pending
	var results []RootResult
	for _, root := range all.Roots {
		if !root.InUse {
			continue
		}
		if outer, ok := within(root.Path, roots); ok && outer != root.Path {
			// The root's files are the outer root's: one walk, one origin per file.
			results = append(results, RootResult{Root: root.Path, State: RootNested})
			continue
		}
		result, waiting, err := im.scanRoot(ctx, root.Path, idx, nil, reports[root.Path])
		// However its walk ended, every one of the root's entries is finished with.
		reports[root.Path].close()
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", root.Path, err)
		}
		im.recordReadability(ctx, result)
		results = append(results, result)
		queue = append(queue, waiting...)
	}
	im.setProgress(func(p *Progress) { p.Roots = results })
	return queue, nil
}

// RunFolders makes a pass over the named folders of one walked root: the ones a change signal
// named. It writes and removes only what is under them, under the same removal guard, and does not
// count as the root's scan. A root that is not walked on its own is left to the full pass.
func (im *Importer) RunFolders(ctx context.Context, root string, dirs []string) error {
	if len(dirs) == 0 || (im.Enabled != nil && !im.Enabled()) {
		return nil
	}
	queue, err := im.importFolders(ctx, root, dirs)
	if err != nil {
		return err
	}
	return im.readHeaders(ctx, queue)
}

// importFolders walks and imports the named folders of one root, and returns the files waiting
// for their header.
func (im *Importer) importFolders(ctx context.Context, root string, dirs []string) ([]pending, error) {
	im.run.Lock()
	defer im.run.Unlock()
	all, err := im.Store.LibraryRoots(ctx)
	if err != nil {
		return nil, err
	}
	if !scannedRoots(all.Roots)[root] {
		return nil, nil
	}
	idx := im.arrIndex(ctx, all.Paths, false)
	im.setProgress(func(p *Progress) { p.ArrIncomplete = !idx.complete })
	// A folder pass is a few folders a change named: it is not step 1 of an import, and shows
	// only the header reads it queues.
	result, queue, err := im.scanRoot(ctx, root, idx, dirs, nil)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	im.recordReadability(ctx, result)
	im.setProgress(func(p *Progress) {
		for i := range p.Roots {
			if p.Roots[i].Root == root {
				// The names it could not read outside the folders it walked still stand.
				for _, rel := range p.Roots[i].Unreadable {
					if !under(filepath.Join(root, filepath.FromSlash(rel)), dirs) {
						result.Unreadable = append(result.Unreadable, rel)
					}
				}
				p.Roots[i] = result
			}
		}
	})
	return queue, nil
}

// ArrChanged notes that an arr changed item, or changed files without naming the item (nil): the
// next folder pass reads it again, or lists every arr again, before trusting its index.
func (im *Importer) ArrChanged(item *ArrItem) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if item == nil {
		im.relist = true
		return
	}
	im.arrChanged = append(im.arrChanged, *item)
}

// arrIndex is what the arrs say each local file is. A full pass lists every arr, as does a folder
// pass with no index yet, an index from a listing an arr failed, a changed path map, or a change
// it cannot read alone. Otherwise a folder pass reuses the last listing, reading again just the
// items an arr said changed: Sonarr's listing costs two requests per series. Call it holding run.
func (im *Importer) arrIndex(ctx context.Context, paths []librarystore.LibraryRootPath, full bool) managerIndex {
	im.mu.Lock()
	changed, relist := im.arrChanged, im.relist
	im.arrChanged, im.relist = nil, false
	im.mu.Unlock()

	var managers []Manager
	if im.Managers != nil {
		managers = im.Managers()
	}
	key := fmt.Sprint(paths)
	for _, m := range managers {
		if m.Address != nil {
			key += "|" + m.Authority + "=" + m.Address()
		}
	}
	retry := im.index != nil && !im.index.complete && im.now().Sub(im.indexedAt) >= arrRetry
	if full || relist || im.index == nil || retry || im.indexPaths != key {
		return im.listArrs(ctx, managers, paths, key)
	}
	if !im.index.complete {
		// An arr failed a moment ago: asking it again on every folder pass only waits on it
		// again. Its folders' files wait; the rest pass on what the others listed.
		return *im.index
	}
	for _, item := range changed {
		if !im.refreshArrItem(ctx, managers, paths, item) {
			return im.listArrs(ctx, managers, paths, key)
		}
	}
	return *im.index
}

// arrWithoutFolders reports whether a connected arr has no folder in the path map.
func (im *Importer) arrWithoutFolders(paths []librarystore.LibraryRootPath) bool {
	if im.Managers == nil {
		return false
	}
	for _, m := range im.Managers() {
		if m.Address == nil || m.Address() == "" {
			continue
		}
		if !slices.ContainsFunc(paths, func(p librarystore.LibraryRootPath) bool { return p.Authority == m.Authority }) {
			return true
		}
	}
	return false
}

func (im *Importer) listArrs(ctx context.Context, managers []Manager, paths []librarystore.LibraryRootPath, key string) managerIndex {
	idx, errs := indexManagers(ctx, managers, paths)
	for _, err := range errs {
		im.log().Warn("library scan: an arr did not list", "err", err)
	}
	im.index, im.indexPaths, im.indexedAt = &idx, key, im.now()
	return idx
}

// refreshArrItem reads one changed item into the index. It reports false when it cannot, and the
// arr must be listed whole instead.
func (im *Importer) refreshArrItem(ctx context.Context, managers []Manager, paths []librarystore.LibraryRootPath, item ArrItem) bool {
	for _, m := range managers {
		if m.Authority != item.Authority {
			continue
		}
		if m.Fetcher == nil {
			return false
		}
		rec, err := m.Fetcher.Fetch(ctx, item.Ref)
		switch {
		case errors.Is(err, mp.ErrNotFound):
			im.index.refresh(m.Authority, pathMapper(m.Authority, paths), item.Ref, nil)
		case err != nil:
			im.log().Warn("library scan: an arr did not answer for one item; listing it whole",
				"arr", m.Authority, "err", err)
			return false
		default:
			im.index.refresh(m.Authority, pathMapper(m.Authority, paths), item.Ref, &rec)
		}
		return true
	}
	return true // an arr no longer configured manages nothing
}

// recordReadability keeps the root's readability, and whether it held anything, as the pass found
// them, for the status strip. It tells RootsChanged when either changed and RootReadability when
// readability did. A failed write is logged: the pass's own work stands.
func (im *Importer) recordReadability(ctx context.Context, result RootResult) {
	readable := result.State != RootUnreadable
	check := Folder{Readable: readable, Empty: result.Empty}.check(result.Root)
	flipped, err := im.Store.CheckLibraryRoot(ctx, check, im.now())
	if err != nil {
		if ctx.Err() == nil {
			im.log().Warn("library scan: could not record a folder's readability", "root", result.Root, "err", err)
		}
		return
	}
	if (flipped.Readable || flipped.Empty) && im.RootsChanged != nil {
		im.RootsChanged()
	}
	if flipped.Empty && check.Readable {
		if check.Empty {
			im.log().Warn("library scan: a media folder opens but holds nothing; is its drive connected?", "root", result.Root)
		} else {
			im.log().Info("library scan: a media folder that held nothing has files again", "root", result.Root)
		}
	}
	if !flipped.Readable {
		return
	}
	if readable {
		im.log().Info("library scan: a media folder opens again", "root", result.Root)
	} else {
		im.log().Warn("library scan: a media folder stopped opening; nothing in it is marked missing", "root", result.Root)
	}
	if im.RootReadability != nil {
		im.RootReadability(ctx, result.Root, readable)
	}
}

// scannedRoots is the in-use roots a pass walks: each one not inside another in-use root, by real
// path. A folder inside another is walked as part of it.
func scannedRoots(all []librarystore.LibraryRoot) map[string]bool {
	real := map[string]string{}
	for _, r := range all {
		if !r.InUse {
			continue
		}
		p, err := filepath.EvalSymlinks(r.Path)
		if err != nil {
			p = r.Path
		}
		real[r.Path] = p
	}
	out := map[string]bool{}
	for path, p := range real {
		nested := false
		for other, q := range real {
			if other != path && (p == q && other < path || strings.HasPrefix(p, q+string(filepath.Separator))) {
				nested = true
				break
			}
		}
		if !nested {
			out[path] = true
		}
	}
	return out
}

// within reports the walked root that contains root, root itself when it is walked.
func within(root string, walked map[string]bool) (string, bool) {
	if walked[root] {
		return root, true
	}
	p, err := filepath.EvalSymlinks(root)
	if err != nil {
		p = root
	}
	for outer := range walked {
		q, err := filepath.EvalSymlinks(outer)
		if err != nil {
			q = outer
		}
		if p == q || strings.HasPrefix(p, q+string(filepath.Separator)) {
			return outer, true
		}
	}
	return "", false
}

// forgetReleased marks every file of a root no longer walked unseen and forgets its rows: releasing
// a root is the maintainer's word, not a walk's guess, so the removal guard does not apply.
func (im *Importer) forgetReleased(ctx context.Context, inUse map[string]bool) error {
	scanned, err := im.Store.LibraryFileRoots(ctx)
	if err != nil {
		return err
	}
	for _, root := range scanned {
		if inUse[root] {
			continue
		}
		if err := im.Inventory.MarkUnseen(ctx, Authority(root), im.now(), nil); err != nil {
			return err
		}
		if err := im.Store.DeleteLibraryFiles(ctx, root); err != nil {
			return err
		}
		im.titlesChanged()
	}
	return nil
}

// scanRoot walks one root, or only the folders of it in scope when scope is set, and writes what
// changed. It returns the files whose headers are unread. report, when not nil, hears how the
// walk of the whole root goes.
func (im *Importer) scanRoot(ctx context.Context, root string, idx managerIndex, scope []string, report *walkReport) (RootResult, []pending, error) {
	im.gen++
	gen := im.gen
	var save []librarystore.LibraryFile
	// On every return, an error included: a file applied before the error was still written.
	defer func() { im.touch(save, gen) }()
	result := RootResult{Root: root, State: RootScanned}
	if im.Disk != nil {
		look := im.Disk.Look(ctx, root)
		if !look.Readable {
			result.State = RootUnreadable
			return result, nil, nil
		}
		result.Empty = look.Empty
	}
	if im.stalled(root) {
		// The last walk is still blocked on this share: it reads as offline, and nothing in it
		// is marked missing.
		result.State = RootUnreadable
		return result, nil, nil
	}
	walk, done, err := watchedWalk(ctx, orDuration(im.WalkStall, walkStall), func(steps *atomic.Int64) (Walk, error) {
		if scope == nil {
			return walkRoot(ctx, root, steps, report)
		}
		scope = existingFolders(root, scope)
		return walkFolders(ctx, root, scope, steps)
	})
	if errors.Is(err, errWalkStalled) {
		im.log().Warn("library scan: a media folder stopped answering mid-walk; nothing in it is marked missing", "root", root)
		im.markStalled(root, done)
		result.State = RootUnreadable
		return result, nil, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return result, nil, ctx.Err()
		}
		result.State = RootUnreadable
		return result, nil, nil
	}
	if !walk.Complete {
		result.State = RootIncomplete
	}
	for _, path := range walk.Skipped {
		im.log().Warn("library scan: a name is not UTF-8 and was skipped", "path", path)
		result.Unreadable = append(result.Unreadable, strings.ToValidUTF8(relTo(root, path), "�"))
	}
	rows, err := im.Store.LibraryFiles(ctx, root)
	if err != nil {
		return result, nil, err
	}
	prev := make(map[string]librarystore.LibraryFile, len(rows))
	for _, r := range rows {
		prev[r.Path] = r
	}

	now := im.now()
	b := newBuilder(im, root, idx)
	var queue []pending
	var episodes []episodeRow
	found := map[string]bool{}
	for _, file := range walk.Files {
		found[file.Path] = true
	}
	// A folder or file the walk couldn't read is out of scope too: whatever it holds stands as it was,
	// while a file gone from anywhere the walk did read is still gone.
	inScope := func(path string) bool { return (scope == nil || under(path, scope)) && !under(path, walk.Unread) }
	moved := movedFrom(prev, found, inScope)
	// A pass over some folders still sees every present file outside them, so marking unseen
	// below takes only what is gone from the folders it walked.
	var others []identity
	for _, r := range rows {
		if inScope(r.Path) || !r.MissingAt.IsZero() || r.Problem == ProblemDiscImage {
			continue
		}
		if rel, err := filepath.Rel(root, r.Path); err == nil {
			others = append(others, b.identify(File{Path: r.Path, Rel: filepath.ToSlash(rel)}))
		}
	}
	// Every file is identified before any is written, so an episode's series knows each season on
	// disk when the episode is described (decision 53), and a show kept in several places is one.
	ids := make([]identity, len(walk.Files))
	for i, file := range walk.Files {
		if file.Problem == "" {
			ids[i] = b.identify(file)
		}
	}
	b.joinShows(ids, others)
	// The walk is over; the writes below are the other half of step 1, and can take as long.
	report.writing()
	writingDir := ""
	for i, file := range walk.Files {
		// The entries before this file's are written, and the step names the folder it is on.
		report.wrote(file.Top)
		if dir := relDir(file.Rel); dir != writingDir {
			writingDir = dir
			report.listing(dir)
		}
		old, known := prev[file.Path]
		// A held file the walk finds again is back.
		old.Held = false
		if file.Problem != "" {
			if !known || old.Problem != file.Problem || !old.MissingAt.IsZero() || prev[file.Path].Held {
				save = append(save, librarystore.LibraryFile{Path: file.Path, Root: root, Size: file.Stat.Size,
					ModifiedNs: file.Stat.ModifiedNs, Header: librarystore.LibraryHeaderFailed, Problem: file.Problem, SeenAt: now})
			}
			continue
		}
		id := ids[i]
		row := librarystore.LibraryFile{Path: file.Path, Root: root, Size: file.Stat.Size, ModifiedNs: file.Stat.ModifiedNs,
			Identity: id.digest, Kind: string(kindOf(id.kind)), Header: librarystore.LibraryHeaderPending, SeenAt: now}
		// The same bytes as the row recorded, whether the file stayed or came back as it went (a
		// drive plugged in again after its titles were let go): the header read then is still its
		// own. Only one that stayed is unchanged; one that came back is written present again.
		sameBytes := known && old.Size == file.Stat.Size && old.ModifiedNs == file.Stat.ModifiedNs
		if sameBytes {
			row.Header = old.Header
		}
		switch {
		case sameBytes && old.MissingAt.IsZero() && old.Identity == id.digest && old.ItemID != "":
			// Unchanged: never written, never opened again.
			if prev[file.Path].Held {
				save = append(save, old)
			}
			if old.Header == librarystore.LibraryHeaderPending && old.Problem == "" {
				queue = append(queue, pending{row: old, file: file, id: id, b: b, gen: gen})
			}
			continue
		case !idx.knows(file.Path):
			// An arr that manages this folder is down: what it would say about a new, changed or
			// re-identified file is unknown, and writing without it would only have to be undone.
			// The file waits. A folder no failing arr manages carries on.
			if prev[file.Path].Held {
				save = append(save, old)
			}
			continue
		}
		b.markWritten(id)
		snapshot := b.snapshot(ctx, file, id, now)
		var read *inventory.SourceFacts
		var readBy int
		if sameBytes && old.Header == librarystore.LibraryHeaderRead {
			read, readBy = im.lastHeader(ctx, snapshot.Origin)
			if read == nil {
				row.Header = librarystore.LibraryHeaderPending
			}
		} else if from, ok := moved[file.Stat]; ok && !sameBytes {
			// Renamed or moved: the header read at its old path is this file's, and it is not
			// opened again.
			delete(moved, file.Stat)
			if rel, err := filepath.Rel(root, from.Path); err == nil {
				if read, readBy = im.lastHeader(ctx, fileOrigin(root, filepath.ToSlash(rel))); read != nil {
					row.Header = librarystore.LibraryHeaderRead
				}
			}
		}
		// An older reader's facts stand, and the title stays ready, until this reader reads the file
		// again; till then the file keeps its old identity, so a pass cut short comes back to it.
		reread := read != nil && readBy < im.reader()
		snapshot.Sources = []inventory.SourceSnapshot{sourceSnapshot(file, read, readBy, now)}
		itemID, problem, err := im.applyFile(ctx, snapshot)
		if err != nil {
			return result, nil, err
		} else if problem != "" {
			// No digest: a file that could not be written is tried again on the next pass.
			row.Problem, row.Identity = problem, ""
			save = append(save, row)
			continue
		}
		row.ItemID = itemID
		if snapshot.Kind == kindMovie {
			b.listed(ctx, itemID, snapshot)
		}
		if id.seriesDir != "" {
			episodes = append(episodes, episodeRow{at: len(save), seriesDir: id.seriesDir})
		}
		saved := row
		if reread {
			saved.Identity = old.Identity
		}
		save = append(save, saved)
		if row.Header == librarystore.LibraryHeaderPending || reread {
			queue = append(queue, pending{row: row, file: file, id: id, b: b, gen: gen})
		}
	}
	if err := b.writeSeries(ctx, now); err != nil {
		return result, nil, err
	}
	// An episode written this pass belongs to the series item writeSeries wrote for its folder.
	for _, ep := range episodes {
		if s := b.series[ep.seriesDir]; s != nil {
			save[ep.at].SeriesID = s.itemID
		}
	}
	// The header stage saves its own copy of each row it reads.
	for i := range queue {
		if s := b.series[queue[i].id.seriesDir]; s != nil && queue[i].row.SeriesID == "" {
			queue[i].row.SeriesID = s.itemID
		}
	}
	removed, present := removals(prev, found, inScope)
	// While an arr fails to list nothing is marked missing: a file it renamed meanwhile waits at its
	// new path, and marking the old one missing would take the title off the air until it answers.
	if idx.complete {
		// A confirmation is spent by the first complete walk of the whole root after it, held or
		// not: it answers the removal the maintainer saw, never a later one.
		confirmed := scope == nil && walk.Complete && im.takeConfirmed(root)
		// Only importable files count toward the hold, as they do toward what the root holds.
		importable := 0
		for _, r := range removed {
			if r.Problem == "" {
				importable++
			}
		}
		over := importable > holdAtLeast || (present > 0 && float64(importable) > holdShare*float64(present))
		held := over && !confirmed
		for _, r := range removed {
			if held && r.Problem == "" {
				if !r.Held {
					r.Held = true
					save = append(save, r)
				}
				continue
			}
			r.Held, r.MissingAt = false, now
			save = append(save, r)
		}
		if held {
			result.State, result.Held = RootHeld, importable
		}
	}
	if err := im.Store.SaveLibraryFiles(ctx, save); err != nil {
		return result, nil, err
	}
	if len(save) > 0 {
		im.titlesChanged()
	}
	// Origins are marked unseen only with no removal held and every arr answering: a file that waited
	// for an arr was not written, and its series not seen. What an unread folder holds was identified
	// above, so it is seen.
	if result.State != RootHeld && idx.complete {
		if err := im.markUnseen(ctx, root, b.seen, now); err != nil {
			return result, nil, err
		}
		im.titlesChanged()
		if scope == nil && walk.Complete {
			if err := im.Store.SetLibraryRootScanned(ctx, root, now); err != nil && !errors.Is(err, store.ErrNotFound) {
				return result, nil, err
			}
			// The folder answered in full: its trash is emptied of copies removed long enough ago.
			if err := emptyTrash(root, now); err != nil {
				im.log().Warn("library scan: couldn't empty a media folder's trash", "root", root, "err", err)
			}
		}
	}
	return result, queue, nil
}

// applyFile writes one file's item and returns its id. A snapshot the inventory refuses is a
// problem on that file, not a failed pass.
func (im *Importer) applyFile(ctx context.Context, snapshot inventory.Snapshot) (string, string, error) {
	id, err := im.apply(ctx, snapshot)
	switch {
	case err == nil:
		return string(id), "", nil
	case errors.Is(err, inventory.ErrIdentityConflict):
		return "", ProblemIdentityConflict, nil
	case errors.Is(err, inventory.ErrInvalid):
		im.log().Warn("library scan: a file's facts were refused", "err", err)
		return "", ProblemInvalid, nil
	default:
		return "", "", err
	}
}

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

// apply writes one item. Every item a pass or a header read writes goes through here.
func (im *Importer) apply(ctx context.Context, snapshot inventory.Snapshot) (inventory.ItemID, error) {
	id, err := im.Inventory.ApplySnapshot(ctx, snapshot)
	if err == nil {
		im.titlesChanged()
	}
	return id, err
}

// lastHeader is the header facts the inventory holds for a file origin and the header.Version that
// read them; nil when it holds none.
func (im *Importer) lastHeader(ctx context.Context, origin inventory.OriginKey) (*inventory.SourceFacts, int) {
	item, ok, err := im.Inventory.Item(ctx, inventory.ItemRef{Origin: &origin})
	if err != nil || !ok {
		return nil, 0
	}
	for _, src := range item.Sources {
		for _, o := range src.Origins {
			if o.Key == origin && o.Observation.Coverage["streams"] == inventory.CoveragePresent {
				facts := o.Observation.Facts
				return &facts, o.Observation.SchemaVersion
			}
		}
	}
	return nil, 0
}

// reader is the header.Version this importer's header reads are recorded under.
func (im *Importer) reader() int { return cmp.Or(im.readerVersion, header.Version) }

func (im *Importer) now() time.Time {
	if im.Now != nil {
		return im.Now().UTC()
	}
	return time.Now().UTC()
}

func (im *Importer) log() *slog.Logger {
	if im.Log != nil {
		return im.Log
	}
	return slog.Default()
}
