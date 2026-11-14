package scan

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Pass timing (decision 9).
const (
	// ReconcileInterval is how often a full pass runs with no other reason to.
	ReconcileInterval = time.Hour
	// FolderDebounce is how long one folder's changes must be quiet before it is passed over. A
	// file being copied in keeps its folder busy, so a pass rarely reads half a file; other
	// folders are not held back by it.
	FolderDebounce = 5 * time.Second
	// FolderMaxWait is the longest a folder's changes wait while it stays busy. A pass that reads
	// a file still being copied in sees it change again when the copy ends, and passes again.
	FolderMaxWait = 2 * time.Minute
)

// Scanner runs the importer: once at start, then every interval, and whenever asked. A request
// while a pass runs queues exactly one more pass. Each folder a change signal names (the watcher,
// an arr's webhook) is passed over on its own once its changes stop for Debounce, or have waited
// MaxWait.
type Scanner struct {
	Importer *Importer
	Interval time.Duration
	// Debounce is zero for FolderDebounce, MaxWait zero for FolderMaxWait.
	Debounce time.Duration
	MaxWait  time.Duration
	trigger  chan struct{}

	// AfterFolders hears the end of each round of folder passes; nil hears nothing. It must not
	// block.
	AfterFolders func()

	mu      sync.Mutex
	folders map[string]map[string]folderWait // root → changed folders
	// passing is whether a round of folder passes is running. Guarded by mu.
	passing bool
	changed chan struct{}
}

// FoldersSettled reports whether no folder waits for a pass and none is being passed over: every
// pass asked for so far has written what it found.
func (s *Scanner) FoldersSettled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.folders) == 0 && !s.passing
}

// folderWait is when a folder's waiting changes began, and its latest.
type folderWait struct{ first, last time.Time }

// NewScanner returns a scanner running im every interval (ReconcileInterval when zero).
func NewScanner(im *Importer, interval time.Duration) *Scanner {
	if interval <= 0 {
		interval = ReconcileInterval
	}
	return &Scanner{
		Importer: im, Interval: interval, trigger: make(chan struct{}, 1),
		folders: map[string]map[string]folderWait{}, changed: make(chan struct{}, 1),
	}
}

// PassFolders asks for a pass over dirs, under root, once each has been quiet for the debounce. It
// never blocks.
func (s *Scanner) PassFolders(root string, dirs ...string) {
	if len(dirs) == 0 {
		return
	}
	now := time.Now()
	s.mu.Lock()
	if s.folders[root] == nil {
		s.folders[root] = map[string]folderWait{}
	}
	for _, dir := range dirs {
		w := s.folders[root][dir]
		if w.first.IsZero() {
			w.first = now
		}
		w.last = now
		s.folders[root][dir] = w
	}
	s.mu.Unlock()
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// ArrChanged asks for a pass over the folders an arr's event names, given in the arr's own paths:
// each is mapped to a folder here through the path map. A folder that maps nowhere, or outside
// every walked root, is left to the hourly walk. It returns how many folders were queued.
func (s *Scanner) ArrChanged(ctx context.Context, authority string, folders []string) (int, error) {
	if len(folders) == 0 {
		return 0, nil
	}
	all, err := s.Importer.Store.LibraryRoots(ctx)
	if err != nil {
		return 0, err
	}
	local := pathMapper(authority, all.Paths)
	roots := slices.Sorted(maps.Keys(scannedRoots(all.Roots)))
	queued := 0
	for _, folder := range folders {
		dir := local(folder)
		if dir == "" {
			continue
		}
		if root := rootOf(dir, roots); root != "" {
			s.PassFolders(root, dir)
			queued++
		}
	}
	return queued, nil
}

// passFolders passes over each changed folder once it is due, until ctx ends.
func (s *Scanner) passFolders(ctx context.Context) {
	var wake <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.changed:
		case <-wake:
		}
		due, next := s.dueFolders(time.Now())
		for _, root := range slices.Sorted(maps.Keys(due)) {
			if err := s.Importer.RunFolders(ctx, root, due[root]); err != nil && ctx.Err() == nil {
				s.Importer.log().Warn("library scan: a folder pass failed", "root", root, "err", err)
			}
		}
		if len(due) > 0 {
			s.mu.Lock()
			s.passing = false
			s.mu.Unlock()
			if s.AfterFolders != nil {
				s.AfterFolders()
			}
		}
		wake = nil
		if next > 0 {
			wake = time.After(next)
		}
	}
}

// dueFolders takes the folders due at now, by root, sorted: quiet for the debounce, or waiting
// MaxWait. It returns how long until the next of the rest is due, 0 when none waits.
func (s *Scanner) dueFolders(now time.Time) (map[string][]string, time.Duration) {
	debounce := orDuration(s.Debounce, FolderDebounce)
	maxWait := orDuration(s.MaxWait, FolderMaxWait)
	s.mu.Lock()
	defer s.mu.Unlock()
	due := map[string][]string{}
	var next time.Duration
	for root, dirs := range s.folders {
		for dir, w := range dirs {
			at := w.last.Add(debounce)
			if limit := w.first.Add(maxWait); limit.Before(at) {
				at = limit
			}
			if wait := at.Sub(now); wait > 0 {
				if next == 0 || wait < next {
					next = wait
				}
				continue
			}
			due[root] = append(due[root], dir)
			delete(dirs, dir)
		}
		if len(dirs) == 0 {
			delete(s.folders, root)
		}
	}
	for root := range due {
		slices.Sort(due[root])
	}
	// The folders taken are passed over next, so they count as unsettled until that round ends.
	s.passing = len(due) > 0
	return due, next
}

// Trigger asks for a pass soon. It never blocks. The pass is asked for, so it shows its progress
// in full however little it finds.
func (s *Scanner) Trigger() {
	s.Importer.Tracker.Ask()
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Run passes until ctx ends. A failed pass is logged and the next one runs on schedule.
func (s *Scanner) Run(ctx context.Context) {
	var folders sync.WaitGroup
	folders.Go(func() { s.passFolders(ctx) })
	defer folders.Wait()
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	// The first pass of a library with no files yet is its first import, as good as asked for.
	if counts, err := s.Importer.Store.LibraryFileCounts(ctx); err == nil && counts.Files == 0 {
		s.Importer.Tracker.Ask()
	}
	for {
		if err := s.Importer.Run(ctx); err != nil && ctx.Err() == nil {
			s.Importer.log().Warn("library scan: pass failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.trigger:
		}
	}
}

// Library is Settings → Library's service: the folders, and the scanner over them.
type Library struct {
	*Roots
	Scanner *Scanner
	// ItemStore and Inventory read what the scanner wrote, for Library → Items.
	ItemStore librarystore.LibraryItemStore
	Inventory inventory.Service
	// NeedStore keeps Needs you; Search finds titles to answer it with (nil: no catalog).
	NeedStore librarystore.LibraryNeedsStore
	Search    Searcher
	// Orders keeps which order each show's episode files follow (nil: no show is asked about).
	Orders librarystore.LibraryEpisodeOrderStore
	// CollectionStore keeps the household's collections; Records reads cached catalog records: the
	// ready-made sets' own (nil: none are listed), whose movies Keys places by their catalog rows,
	// and a show's episode orders.
	CollectionStore librarystore.LibraryCollectionStore
	Records         RecordReader
	Keys            CatalogKeys
	// Enricher fetches a title's catalog record on request (Update details); nil has no catalog.
	Enricher *Enricher
	Now      func() time.Time
}

// Progress is how the scanner's last passes ended; the zero Progress when no scanner runs.
func (l *Library) Progress() Progress {
	if l.Scanner == nil {
		return Progress{}
	}
	return l.Scanner.Importer.Progress()
}

// Importing is how far a running import is now; the zero snapshot when none runs.
func (l *Library) Importing() ProgressSnapshot {
	if l.Scanner == nil {
		return ProgressSnapshot{}
	}
	return l.Scanner.Importer.Tracker.Snapshot(l.now())
}

// Details is how the titles' details are getting on: how many movies and series still wait for
// theirs, whether they can be looked up now, and when the next try is.
type Details struct {
	Waiting int
	Lookups
}

// Details reads how the titles' details are getting on; the zero wait when there is no catalog.
// RetryAt is the Enricher's next try after a failed lookup while that is still ahead; past it, a
// tick that found every waiting title held back tried nothing, so it is when the first of those is
// tried again. Zero when nothing waits, or when there is no key.
func (l *Library) Details(ctx context.Context) (Details, error) {
	if l.Enricher == nil {
		return Details{Lookups: Lookups{State: LookupsOK}}, nil
	}
	now := l.now()
	waiting, err := l.Enricher.Store.LibraryCatalogWaiting(ctx, now)
	if err != nil {
		return Details{}, err
	}
	d := Details{Waiting: waiting.Titles, Lookups: l.Enricher.Lookups()}
	switch {
	case d.Waiting == 0, d.State == LookupsNoKey:
		// Nothing waits, or nothing is tried again until a key is added.
		d.RetryAt = time.Time{}
	case !d.RetryAt.After(now):
		d.RetryAt = waiting.NextRetry
	}
	return d, nil
}

// Enabled reports whether the kill switch lets the scanner run.
func (l *Library) Enabled() bool {
	if l.Scanner == nil {
		return false
	}
	im := l.Scanner.Importer
	return im.Enabled == nil || im.Enabled()
}

// Counts counts the scanner's files.
func (l *Library) Counts(ctx context.Context) (librarystore.LibraryFileCounts, error) {
	if l.Scanner == nil {
		return librarystore.LibraryFileCounts{}, nil
	}
	return l.Scanner.Importer.Store.LibraryFileCounts(ctx)
}

// skippedListed is how many of the files skipped for one reason the maintainer is shown by path.
const skippedListed = 20

// Skipped lists the files the scan skips, by why (ProblemBrokenLink, ProblemLinkOutside,
// ProblemDiscImage, ...): each reason's count and its first few paths, for the maintainer to fix.
func (l *Library) Skipped(ctx context.Context) ([]librarystore.LibrarySkipped, error) {
	if l.Scanner == nil {
		return []librarystore.LibrarySkipped{}, nil
	}
	return l.Scanner.Importer.Store.LibrarySkippedFiles(ctx, skippedListed)
}

// heldNamed is how many of a held removal's titles the maintainer is shown by name.
const heldNamed = 3

// HeldTitles names a few of the titles a root's held removal would take, by name, and counts them
// all: the maintainer sees what "They're gone, remove them" lets go before saying it.
func (l *Library) HeldTitles(ctx context.Context, root string) ([]string, int, error) {
	if l.Scanner == nil {
		return []string{}, 0, nil
	}
	return l.Scanner.Importer.Store.LibraryHeldTitles(ctx, root, heldNamed)
}

// ErrScannerOff is a request the scanner cannot serve because it is not running.
var ErrScannerOff = errors.New("scan: the scanner is off")

// Rescan asks for a pass now ("Scan now", and "Check again" on a held removal).
func (l *Library) Rescan() error {
	if !l.Enabled() {
		return ErrScannerOff
	}
	l.Scanner.Trigger()
	return nil
}

// SetInUse confirms or releases a root. Confirming one ("Use this folder") asks for a pass, so
// its media starts arriving now rather than at the next hourly pass.
func (l *Library) SetInUse(ctx context.Context, path string, inUse bool) error {
	if err := l.Roots.SetInUse(ctx, path, inUse); err != nil {
		return err
	}
	if inUse && l.Enabled() {
		l.Scanner.Trigger()
	}
	return nil
}

// Add records a folder typed by hand, in use at once, and asks for a pass.
func (l *Library) Add(ctx context.Context, path string) error {
	if err := l.Roots.Add(ctx, path); err != nil {
		return err
	}
	if l.Enabled() {
		l.Scanner.Trigger()
	}
	return nil
}

// Replace adds a folder in place of the folders in use inside it (Roots.Replace), and asks for a
// pass, which reads their files as the outer folder's.
func (l *Library) Replace(ctx context.Context, path string) error {
	if err := l.Roots.Replace(ctx, path); err != nil {
		return err
	}
	if l.Enabled() {
		l.Scanner.Trigger()
	}
	return nil
}

// SetMapping points an authority's folder at a local folder by hand and, when it points somewhere,
// asks for a pass: the files the mapping reaches arrive now, not at the next hourly pass, and no
// restart is needed to make a corrected mapping take (#3359). Clearing one asks for nothing.
func (l *Library) SetMapping(ctx context.Context, authority, authorityPath, local string) error {
	if err := l.Roots.SetMapping(ctx, authority, authorityPath, local); err != nil {
		return err
	}
	if strings.TrimSpace(local) != "" && l.Enabled() {
		l.Scanner.Trigger()
	}
	return nil
}

// RefreshEverything is "Refresh everything", Update details for the whole library: every file is
// written again on a pass asked for now, so what the files, sidecars and arrs say now is read, and
// the enricher fetches every catalog record again. Headers are kept: a file's bytes are read
// again only when they change, so no title stops playing while it runs.
func (l *Library) RefreshEverything(ctx context.Context) error {
	if !l.Enabled() {
		return ErrScannerOff
	}
	if err := l.ItemStore.RefreshLibrary(ctx); err != nil {
		return err
	}
	l.Scanner.Trigger()
	return nil
}

// ConfirmRemovals records the maintainer's word on a root's held removal ("They're gone, remove
// them") and asks for the pass that applies it.
func (l *Library) ConfirmRemovals(ctx context.Context, root string) error {
	if !l.Enabled() {
		return ErrScannerOff
	}
	if err := l.Scanner.Importer.ConfirmRemovals(ctx, root); err != nil {
		return err
	}
	l.Scanner.Trigger()
	return nil
}
