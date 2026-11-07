package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
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
	store.LibraryFileStore
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

	run      sync.Mutex
	mu       sync.Mutex
	progress Progress
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
	var managers []Manager
	if im.Managers != nil {
		managers = im.Managers()
	}
	idx, errs := indexManagers(ctx, managers, all.Paths)
	for _, err := range errs {
		im.log().Warn("library scan: an arr did not list", "err", err)
	}
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
		result, waiting, err := im.scanRoot(ctx, root.Path, idx)
		if err != nil {
			return fmt.Errorf("scan %s: %w", root.Path, err)
		}
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

// scanRoot walks one root and writes what changed. It returns the files whose headers are unread.
func (im *Importer) scanRoot(ctx context.Context, root string, idx managerIndex) (RootResult, []pending, error) {
	result := RootResult{Root: root, State: RootScanned}
	if im.Disk != nil && !im.Disk.Readable(ctx, root) {
		result.State = RootUnreadable
		return result, nil, nil
	}
	walk, err := walkRoot(ctx, root)
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
	found := map[string]bool{}
	for _, file := range walk.Files {
		found[file.Path] = true
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
		case sameBytes && old.Identity == id.digest:
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
		}
		snapshot.Sources = []inventory.SourceSnapshot{sourceSnapshot(file, header, now)}
		if problem, err := im.applyFile(ctx, snapshot); err != nil {
			return result, nil, err
		} else if problem != "" {
			// No digest: a file that could not be written is tried again on the next pass.
			row.Problem, row.Identity = problem, ""
			save = append(save, row)
			continue
		}
		save = append(save, row)
		if row.Header == store.LibraryHeaderPending {
			queue = append(queue, pending{row: row, file: file, id: id, b: b})
		}
	}
	if err := b.writeSeries(ctx, now); err != nil {
		return result, nil, err
	}

	removed, present := removals(prev, found)
	if walk.Complete {
		// A confirmation is spent by the first complete walk after it, held or not: it answers the
		// removal the maintainer saw, never a later one.
		confirmed := im.takeConfirmed(root)
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
		if err := im.Store.SetLibraryRootScanned(ctx, root, now); err != nil && !errors.Is(err, store.ErrNotFound) {
			return result, nil, err
		}
	}
	return result, queue, nil
}

// applyFile writes one file's item. A snapshot the inventory refuses is a problem on that file,
// not a failed pass.
func (im *Importer) applyFile(ctx context.Context, snapshot inventory.Snapshot) (string, error) {
	err := im.apply(ctx, snapshot)
	switch {
	case err == nil:
		return "", nil
	case errors.Is(err, inventory.ErrIdentityConflict):
		return ProblemIdentityConflict, nil
	case errors.Is(err, inventory.ErrInvalid):
		im.log().Warn("library scan: a file's facts were refused", "err", err)
		return ProblemInvalid, nil
	default:
		return "", err
	}
}

// removals is every present file a walk did not find, and how many importable files were present.
// A problem file that is gone is a removal too, but never counts toward a hold.
func removals(prev map[string]store.LibraryFile, found map[string]bool) ([]store.LibraryFile, int) {
	var removed []store.LibraryFile
	present := 0
	for path, r := range prev {
		if !r.MissingAt.IsZero() {
			continue
		}
		if r.Problem == "" {
			present++
		}
		if !found[path] {
			removed = append(removed, r)
		}
	}
	return removed, present
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

func (im *Importer) apply(ctx context.Context, snapshot inventory.Snapshot) error {
	_, err := im.Inventory.ApplySnapshot(ctx, snapshot)
	return err
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
