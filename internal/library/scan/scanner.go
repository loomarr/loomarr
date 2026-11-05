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
	// FolderDebounce is how long folder changes must be quiet before their folders are passed
	// over. A file being copied in keeps them busy, so a pass never reads half a file.
	FolderDebounce = 5 * time.Second
)

// Scanner runs the importer: once at start, then every interval, and whenever asked. A request
// while a pass runs queues exactly one more pass. Folders a change signal names (the watcher, an
// arr's webhook) are passed over on their own once changes stop for Debounce.
type Scanner struct {
	Importer *Importer
	Interval time.Duration
	// Debounce is zero for FolderDebounce.
	Debounce time.Duration
	trigger  chan struct{}

	mu      sync.Mutex
	folders map[string]map[string]bool // root → changed folders
	changed chan struct{}
}

// NewScanner returns a scanner running im every interval (ReconcileInterval when zero).
func NewScanner(im *Importer, interval time.Duration) *Scanner {
	if interval <= 0 {
		interval = ReconcileInterval
	}
	return &Scanner{
		Importer: im, Interval: interval, trigger: make(chan struct{}, 1),
		folders: map[string]map[string]bool{}, changed: make(chan struct{}, 1),
	}
}

// PassFolders asks for a pass over dirs, under root, once folder changes have been quiet for the
// debounce. It never blocks.
func (s *Scanner) PassFolders(root string, dirs ...string) {
	if len(dirs) == 0 {
		return
	}
	s.mu.Lock()
	if s.folders[root] == nil {
		s.folders[root] = map[string]bool{}
	}
	for _, dir := range dirs {
		s.folders[root][dir] = true
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

// passFolders passes over the changed folders each time they go quiet, until ctx ends.
func (s *Scanner) passFolders(ctx context.Context) {
	debounce := orDuration(s.Debounce, FolderDebounce)
	quiet := time.NewTimer(debounce)
	quiet.Stop()
	for {
		select {
		case <-ctx.Done():
			quiet.Stop()
			return
		case <-s.changed:
			quiet.Reset(debounce)
		case <-quiet.C:
			s.mu.Lock()
			folders := s.folders
			s.folders = map[string]map[string]bool{}
			s.mu.Unlock()
			for _, root := range slices.Sorted(maps.Keys(folders)) {
				dirs := slices.Sorted(maps.Keys(folders[root]))
				if err := s.Importer.RunFolders(ctx, root, dirs); err != nil && ctx.Err() == nil {
					s.Importer.log().Warn("library scan: a folder pass failed", "root", root, "err", err)
				}
			}
		}
	}
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
