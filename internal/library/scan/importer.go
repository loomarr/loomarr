package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/localart"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/store"
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

// ImportStore is the store the importer reads roots from and keeps its file table in.
type ImportStore interface {
	LibraryRoots(ctx context.Context) (store.LibraryRoots, error)
	CheckLibraryRoot(ctx context.Context, check store.LibraryRootCheck, at time.Time) (bool, error)
	store.LibraryFileStore
	SaveLibraryItems(ctx context.Context, items []store.LibraryItem, at time.Time) error
}

// Posters puts a title's poster in the image service (#2151 step 10). ref is what precedence
// resolved: an image file beside the media, or a manager's https URL; "" is none. The title owns
// one poster: the one it had before is let go.
type Posters interface {
	Poster(ctx context.Context, itemID, ref string) error
}

// Importer is the shadow scanner (#2151 step 5). Each pass walks every in-use root, writes each
// new or changed media file to the inventory as an origin of its root, then reads the headers of
// files that have none yet. Nothing reads the scanner's origins until the cutover.
type Importer struct {
	Store     ImportStore
	Inventory inventory.Service
	Disk      Disk
	// Managers are the configured arrs, asked on each pass.
	Managers func() []Manager
	// NFO reads sidecars; nil reads none.
	NFO mp.Provider
	// Art lists artwork sidecars; nil lists none. A file's come from the walk's own listing; a
	// series folder's are listed when the series is written.
	Art *localart.Provider
	// Posters imports the poster of each title the pass writes; nil imports none.
	Posters Posters
	// Catalog is the cache of catalog records a title's details come from; nil reads none.
	Catalog CatalogStore
	// Answers is the maintainer's Needs you answers; nil reads none.
	Answers AnswerStore
	// Resolver applies precedence.
	Resolver *precedence.Resolver
	// Header reads one file's container header.
	Header func(ctx context.Context, path string) (header.Header, error)
	// Busy reports whether a playout session is live; header reads wait while it is.
	Busy func() bool
	// Enabled is the kill switch, library.scanner.enabled.
	Enabled func() bool
	// Wanted are ids a channel or proposal already asked for; their files' headers are read first.
	Wanted func(ctx context.Context) []inventory.ExternalID
	Now    func() time.Time
	Log    *slog.Logger
	// Poll is the wait between busy checks; zero means 5 s.
	Poll time.Duration
	// WalkStall is how long a walk may make no progress before the pass gives up on it; zero
	// means walkStall.
	WalkStall time.Duration
	// RootReadability hears when a pass finds a root stopped opening, or opens again (decision
	// 32). Nil hears nothing.
	RootReadability func(ctx context.Context, root string, readable bool)

	run sync.Mutex
	// index is the arr index the last listing built, reused by folder passes, and indexPaths the
	// path map and arr addresses it was built with. Guarded by run.
	index      *managerIndex
	indexPaths string

	mu       sync.Mutex
	progress Progress
	// arrChanged are arr items to read again before the next folder pass uses the index; relist
	// asks for every arr to be listed again. Guarded by mu.
	arrChanged []ArrItem
	relist     bool
	// stalledWalks are roots whose walk was given up on, until it ends. Guarded by mu.
	stalledWalks map[string]<-chan struct{}
	// confirmed are roots whose held removal the maintainer confirmed, applied by their next pass.
	confirmed map[string]bool
}

// Stage is what a pass is doing.
type Stage string

const (
	StageIdle    Stage = "idle"
	StageWalking Stage = "walking"
	StageReading Stage = "reading"
)

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
	// Held is how many files the walk no longer found while they wait for confirmation.
	Held int
}

// Progress is the scanner's state for the status endpoint.
type Progress struct {
	Stage       Stage
	HeadersDone int
	HeadersAll  int
	// Waiting is true while header reads wait for live playout to end.
	Waiting     bool
	StartedAt   time.Time
	CompletedAt time.Time
	Roots       []RootResult
	// ArrIncomplete is true when an arr failed to list on the last pass.
	ArrIncomplete bool
}

// Progress returns the scanner's current state.
func (im *Importer) Progress() Progress {
	im.mu.Lock()
	defer im.mu.Unlock()
	p := im.progress
	p.Roots = append([]RootResult(nil), p.Roots...)
	if p.Stage == "" {
		p.Stage = StageIdle
	}
	return p
}

func (im *Importer) setProgress(update func(*Progress)) {
	im.mu.Lock()
	defer im.mu.Unlock()
	update(&im.progress)
}

// pending is a file waiting for its header. Its item is built when the header is read, so a file
// that waits pass after pass has its sidecar read once, by the read that writes it.
type pending struct {
	row  store.LibraryFile
	file File
	id   identity
	b    *builder
}

// episodeRow is an episode's row in a pass's save list, and its series folder.
type episodeRow struct {
	at        int
	seriesDir string
}

// Run makes one pass. Passes never overlap: a second caller waits for the first.
func (im *Importer) Run(ctx context.Context) error {
	if im.Enabled != nil && !im.Enabled() {
		return nil
	}
	im.run.Lock()
	defer im.run.Unlock()
	now := im.now()
	im.setProgress(func(p *Progress) {
		*p = Progress{Stage: StageWalking, StartedAt: now, CompletedAt: p.CompletedAt}
	})
	defer im.setProgress(func(p *Progress) { p.Stage = StageIdle })

	all, err := im.Store.LibraryRoots(ctx)
	if err != nil {
		return err
	}
	idx := im.arrIndex(ctx, all.Paths, true)
	im.setProgress(func(p *Progress) { p.ArrIncomplete = !idx.complete })
	roots := scannedRoots(all.Roots)
	if err := im.forgetReleased(ctx, roots); err != nil {
		return err
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
		result, waiting, err := im.scanRoot(ctx, root.Path, idx, nil)
		if err != nil {
			return fmt.Errorf("scan %s: %w", root.Path, err)
		}
		im.recordReadability(ctx, result)
		results = append(results, result)
		queue = append(queue, waiting...)
	}
	im.setProgress(func(p *Progress) { p.Roots = results })
	if err := im.readHeaders(ctx, queue); err != nil {
		return err
	}
	im.setProgress(func(p *Progress) { p.CompletedAt = im.now() })
	return nil
}

// RunFolders makes a pass over the named folders of one walked root: the ones a change signal
// named. It writes and removes only what is under them, under the same removal guard, and does not
// count as the root's scan. A root that is not walked on its own is left to the full pass.
func (im *Importer) RunFolders(ctx context.Context, root string, dirs []string) error {
	if len(dirs) == 0 || (im.Enabled != nil && !im.Enabled()) {
		return nil
	}
	im.run.Lock()
	defer im.run.Unlock()
	all, err := im.Store.LibraryRoots(ctx)
	if err != nil {
		return err
	}
	if !scannedRoots(all.Roots)[root] {
		return nil
	}
	idx := im.arrIndex(ctx, all.Paths, false)
	im.setProgress(func(p *Progress) { p.Stage, p.ArrIncomplete = StageWalking, !idx.complete })
	defer im.setProgress(func(p *Progress) { p.Stage = StageIdle })
	result, queue, err := im.scanRoot(ctx, root, idx, dirs)
	if err != nil {
		return fmt.Errorf("scan %s: %w", root, err)
	}
	im.recordReadability(ctx, result)
	im.setProgress(func(p *Progress) {
		for i := range p.Roots {
			if p.Roots[i].Root == root {
				p.Roots[i] = result
			}
		}
	})
	return im.readHeaders(ctx, queue)
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
func (im *Importer) arrIndex(ctx context.Context, paths []store.LibraryRootPath, full bool) managerIndex {
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
	if full || relist || im.index == nil || !im.index.complete || im.indexPaths != key {
		return im.listArrs(ctx, managers, paths, key)
	}
	for _, item := range changed {
		if !im.refreshArrItem(ctx, managers, paths, item) {
			return im.listArrs(ctx, managers, paths, key)
		}
	}
	return *im.index
}

func (im *Importer) listArrs(ctx context.Context, managers []Manager, paths []store.LibraryRootPath, key string) managerIndex {
	idx, errs := indexManagers(ctx, managers, paths)
	for _, err := range errs {
		im.log().Warn("library scan: an arr did not list", "err", err)
	}
	im.index, im.indexPaths = &idx, key
	return idx
}

// refreshArrItem reads one changed item into the index. It reports false when it cannot, and the
// arr must be listed whole instead.
func (im *Importer) refreshArrItem(ctx context.Context, managers []Manager, paths []store.LibraryRootPath, item ArrItem) bool {
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

// recordReadability keeps the root's readability as the pass found it, for the status strip, and
// tells RootReadability when it changed. A failed write is logged: the pass's own work stands.
func (im *Importer) recordReadability(ctx context.Context, result RootResult) {
	readable := result.State != RootUnreadable
	flipped, err := im.Store.CheckLibraryRoot(ctx, store.LibraryRootCheck{Path: result.Root, Readable: readable}, im.now())
	if err != nil {
		if ctx.Err() == nil {
			im.log().Warn("library scan: could not record a folder's readability", "root", result.Root, "err", err)
		}
		return
	}
	if !flipped {
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
func scannedRoots(all []store.LibraryRoot) map[string]bool {
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
	}
	return nil
}

// scanRoot walks one root, or only the folders of it in scope when scope is set, and writes what
// changed. It returns the files whose headers are unread.
func (im *Importer) scanRoot(ctx context.Context, root string, idx managerIndex, scope []string) (RootResult, []pending, error) {
	result := RootResult{Root: root, State: RootScanned}
	if im.Disk != nil && !im.Disk.Readable(ctx, root) {
		result.State = RootUnreadable
		return result, nil, nil
	}
	if im.stalled(root) {
		// The last walk is still blocked on this share: it reads as offline, and nothing in it
		// is marked missing.
		result.State = RootUnreadable
		return result, nil, nil
	}
	walk, done, err := watchedWalk(ctx, orDuration(im.WalkStall, walkStall), func(steps *atomic.Int64) (Walk, error) {
		if scope == nil {
			return walkRoot(ctx, root, steps)
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
	}
	rows, err := im.Store.LibraryFiles(ctx, root)
	if err != nil {
		return result, nil, err
	}
	prev := make(map[string]store.LibraryFile, len(rows))
	for _, r := range rows {
		prev[r.Path] = r
	}

	now := im.now()
	b := newBuilder(im, root, idx)
	var save []store.LibraryFile
	var queue []pending
	var episodes []episodeRow
	found := map[string]bool{}
	for _, file := range walk.Files {
		found[file.Path] = true
	}
	inScope := func(path string) bool { return scope == nil || under(path, scope) }
	moved := movedFrom(prev, found, inScope)
	// A pass over some folders still sees every present file outside them, so marking unseen
	// below takes only what is gone from the folders it walked.
	for _, r := range rows {
		if inScope(r.Path) || !r.MissingAt.IsZero() || r.Problem == ProblemDiscImage {
			continue
		}
		if rel, err := filepath.Rel(root, r.Path); err == nil {
			b.identify(File{Path: r.Path, Rel: filepath.ToSlash(rel), Extra: r.Kind == string(kindExtra)})
		}
	}
	for _, file := range b.order(walk.Files) {
		old, known := prev[file.Path]
		// A held file the walk finds again is back.
		old.Held = false
		if file.Problem != "" {
			if !known || old.Problem != file.Problem || !old.MissingAt.IsZero() || prev[file.Path].Held {
				save = append(save, store.LibraryFile{Path: file.Path, Root: root, Size: file.Stat.Size,
					ModifiedNs: file.Stat.ModifiedNs, Header: store.LibraryHeaderFailed, Problem: file.Problem, SeenAt: now})
			}
			continue
		}
		id := b.identify(file)
		row := store.LibraryFile{Path: file.Path, Root: root, Size: file.Stat.Size, ModifiedNs: file.Stat.ModifiedNs,
			Identity: id.digest, Kind: string(id.itemKind(file)), Header: store.LibraryHeaderPending, SeenAt: now}
		sameBytes := known && old.Size == file.Stat.Size && old.ModifiedNs == file.Stat.ModifiedNs && old.MissingAt.IsZero()
		if sameBytes {
			row.Header = old.Header
		}
		switch {
		case sameBytes && old.Identity == id.digest && old.ItemID != "":
			// Unchanged: never written, never opened again.
			if prev[file.Path].Held {
				save = append(save, old)
			}
			if old.Header == store.LibraryHeaderPending && old.Problem == "" {
				queue = append(queue, pending{row: old, file: file, id: id, b: b})
			}
			continue
		case !idx.complete:
			// An arr is down: what it would say about a new, changed or re-identified file is
			// unknown, and writing without it would only have to be undone. The file waits.
			if prev[file.Path].Held {
				save = append(save, old)
			}
			continue
		}
		b.markWritten(id)
		snapshot := b.snapshot(ctx, file, id, now)
		var header *inventory.SourceFacts
		if sameBytes && old.Header == store.LibraryHeaderRead {
			header = im.lastHeader(ctx, snapshot.Origin)
			if header == nil {
				row.Header = store.LibraryHeaderPending
			}
		} else if from, ok := moved[file.Stat]; ok && !sameBytes {
			// Renamed or moved: the header read at its old path is this file's, and it is not
			// opened again.
			delete(moved, file.Stat)
			if rel, err := filepath.Rel(root, from.Path); err == nil {
				if header = im.lastHeader(ctx, fileOrigin(root, filepath.ToSlash(rel))); header != nil {
					row.Header = store.LibraryHeaderRead
				}
			}
		}
		snapshot.Sources = []inventory.SourceSnapshot{sourceSnapshot(file, header, now)}
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
		save = append(save, row)
		if row.Header == store.LibraryHeaderPending {
			queue = append(queue, pending{row: row, file: file, id: id, b: b})
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
	if err := im.Store.SaveLibraryItems(ctx, b.items, now); err != nil {
		return result, nil, err
	}

	removed, present := removals(prev, found, inScope)
	if walk.Complete {
		// A confirmation is spent by the first complete walk of the whole root after it, held or
		// not: it answers the removal the maintainer saw, never a later one.
		confirmed := scope == nil && im.takeConfirmed(root)
		over := len(removed) > holdAtLeast || (present > 0 && float64(len(removed)) > holdShare*float64(present))
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
			result.State, result.Held = RootHeld, len(removed)
		}
	}
	if err := im.Store.SaveLibraryFiles(ctx, save); err != nil {
		return result, nil, err
	}
	// Origins are marked unseen only after a complete walk, with no removal held and every arr
	// answering: a file that waited for an arr was not written, and its series not seen.
	if walk.Complete && result.State != RootHeld && idx.complete {
		if err := im.markUnseen(ctx, root, b.seen, now); err != nil {
			return result, nil, err
		}
		if scope == nil {
			if err := im.Store.SetLibraryRootScanned(ctx, root, now); err != nil && !errors.Is(err, store.ErrNotFound) {
				return result, nil, err
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

// removals is every present file in the walk's scope that it did not find, and how many
// importable files the root held. A problem file that is gone is a removal too, but never counts
// toward a hold.
// movedFrom is each file this pass no longer finds whose header was read, by its size and
// modification time: a new path with the same pair is the same bytes renamed or moved, as Sonarr
// and Radarr do on every rename. A pair two vanished files share names neither.
func movedFrom(prev map[string]store.LibraryFile, found map[string]bool, inScope func(string) bool) map[Stat]store.LibraryFile {
	out := map[Stat]store.LibraryFile{}
	shared := map[Stat]bool{}
	for path, r := range prev {
		if found[path] || !inScope(path) || !r.MissingAt.IsZero() || r.Problem != "" || r.Header != store.LibraryHeaderRead {
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

func removals(prev map[string]store.LibraryFile, found map[string]bool, inScope func(string) bool) ([]store.LibraryFile, int) {
	var removed []store.LibraryFile
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

func (im *Importer) apply(ctx context.Context, snapshot inventory.Snapshot) (inventory.ItemID, error) {
	return im.Inventory.ApplySnapshot(ctx, snapshot)
}

// lastHeader is the header facts the inventory holds for a file origin, nil when it holds none.
func (im *Importer) lastHeader(ctx context.Context, origin inventory.OriginKey) *inventory.SourceFacts {
	item, ok, err := im.Inventory.Item(ctx, inventory.ItemRef{Origin: &origin})
	if err != nil || !ok {
		return nil
	}
	for _, src := range item.Sources {
		for _, o := range src.Origins {
			if o.Key == origin && o.Observation.Coverage["streams"] == inventory.CoveragePresent {
				facts := o.Observation.Facts
				return &facts
			}
		}
	}
	return nil
}

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
