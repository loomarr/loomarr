package scan

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// The Needs you notice (#2151 decision 32): the operators hear, at most once a day, how many titles
// Loomarr can't tell what they are, when any are new since the last notice. A title a notice
// counted is marked in the store, so a restart neither repeats a notice nor loses one.

const (
	// NeedsNoticeInterval is how often the notice looks for new titles.
	NeedsNoticeInterval = time.Hour
	// needsNoticeGap is the least time between two notices.
	needsNoticeGap = 24 * time.Hour
)

// NeedsNoticeStore is what the notice reads and marks.
type NeedsNoticeStore interface {
	LibraryNeeds(ctx context.Context) ([]librarystore.LibraryNeed, error)
	UnnoticedLibraryNeeds(ctx context.Context) ([]string, error)
	NoticeLibraryNeeds(ctx context.Context, itemIDs []string) error
}

// NeedsNotice sends the Needs you notice.
type NeedsNotice struct {
	Store NeedsNoticeStore
	// Notify tells the operators that total titles need them.
	Notify func(ctx context.Context, total int)
	// Settled is true once a pass has finished and none runs: a notice mid-import would count
	// only what the walk had reached. Nil is always settled.
	Settled func() bool
	// Enabled is the scanner's kill switch; nil is on.
	Enabled  func() bool
	Interval time.Duration
	Now      func() time.Time
	Log      *slog.Logger

	last time.Time
}

// Run looks every Interval until ctx ends; the first look is one Interval in.
func (n *NeedsNotice) Run(ctx context.Context) {
	tick := time.NewTicker(orDuration(n.Interval, NeedsNoticeInterval))
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if err := n.Tick(ctx); err != nil && ctx.Err() == nil {
			n.log().Warn("library needs notice: tick failed", "err", err)
		}
	}
}

// Tick sends the notice when titles are new since the last one, and a day has passed since it.
func (n *NeedsNotice) Tick(ctx context.Context) error {
	if (n.Enabled != nil && !n.Enabled()) || (n.Settled != nil && !n.Settled()) {
		return nil
	}
	now := n.now()
	if !n.last.IsZero() && now.Sub(n.last) < needsNoticeGap {
		return nil
	}
	fresh, err := n.Store.UnnoticedLibraryNeeds(ctx)
	if err != nil || len(fresh) == 0 {
		return err
	}
	needs, err := n.Store.LibraryNeeds(ctx)
	if err != nil {
		return err
	}
	n.Notify(ctx, len(needs))
	n.last = now
	return n.Store.NoticeLibraryNeeds(ctx, fresh)
}

func (n *NeedsNotice) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now()
}

func (n *NeedsNotice) log() *slog.Logger {
	if n.Log != nil {
		return n.Log
	}
	return slog.Default()
}
