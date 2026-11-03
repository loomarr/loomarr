package scan

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/store"
)

// ReconcileInterval is how often a full pass runs with no other reason to (decision 9).
const ReconcileInterval = time.Hour

// Scanner runs the importer: once at start, then every interval, and whenever asked. A request
// while a pass runs queues exactly one more pass.
type Scanner struct {
	Importer *Importer
	Interval time.Duration
	trigger  chan struct{}
}

// NewScanner returns a scanner running im every interval (ReconcileInterval when zero).
func NewScanner(im *Importer, interval time.Duration) *Scanner {
	if interval <= 0 {
		interval = ReconcileInterval
	}
	return &Scanner{Importer: im, Interval: interval, trigger: make(chan struct{}, 1)}
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
