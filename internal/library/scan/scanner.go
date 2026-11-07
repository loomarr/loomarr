package scan

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/store"
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

	mu      sync.Mutex
	folders map[string]map[string]folderWait // root → changed folders
	changed chan struct{}
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
	return due, next
}

// Trigger asks for a pass soon. It never blocks.
func (s *Scanner) Trigger() {
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
	ItemStore store.LibraryItemStore
	Inventory inventory.Service
}

// Progress is the scanner's current state; the zero Progress when no scanner runs.
func (l *Library) Progress() Progress {
	if l.Scanner == nil {
		return Progress{Stage: StageIdle}
	}
	return l.Scanner.Importer.Progress()
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
func (l *Library) Counts(ctx context.Context) (store.LibraryFileCounts, error) {
	if l.Scanner == nil {
		return store.LibraryFileCounts{}, nil
	}
	return l.Scanner.Importer.Store.LibraryFileCounts(ctx)
}

// ErrScannerOff is a request the scanner cannot serve because it is not running.
var ErrScannerOff = errors.New("scan: the scanner is off")

// Rescan asks for a pass now ("Check again").
func (l *Library) Rescan() error {
	if !l.Enabled() {
		return ErrScannerOff
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
