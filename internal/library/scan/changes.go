package scan

import (
	"context"
	"errors"
	"log/slog"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// ChangeFollower defaults (#2151 decision 20).
const (
	// ChangeInterval is how often the ChangeFollower checks whether the feed is due.
	ChangeInterval = time.Hour
	// ChangeEvery is how often the feed is read: nightly.
	ChangeEvery = 24 * time.Hour
)

// ChangeStore is what the ChangeFollower reads and marks.
type ChangeStore interface {
	MarkLibraryCatalogChanged(ctx context.Context, keys []librarystore.LibraryCatalogKey) (int, error)
	LibraryCatalogFeedRead(ctx context.Context, provider string) (time.Time, bool, error)
	SaveLibraryCatalogFeedRead(ctx context.Context, provider string, at time.Time) error
}

// ChangeFollower reads a catalog's change feed nightly (#3225, #2151 decision 20) and marks the
// cached records it names as changed, so the Enricher fetches them again ahead of the 90-day
// refresh: a corrected poster, overview or rating reaches the Library within a day. Where it
// last read through is kept in the store, so a restart picks up there.
type ChangeFollower struct {
	Store ChangeStore
	Feed  mp.ChangeFeed
	// Catalog is the provider the feed's records are cached under ("tmdb").
	Catalog string
	// Enabled is the scanner's kill switch; nil is on. Off, the feed is not read.
	Enabled  func() bool
	Interval time.Duration
	Every    time.Duration
	Now      func() time.Time
	Log      *slog.Logger
}

// Run reads the feed when it is due until ctx ends.
func (c *ChangeFollower) Run(ctx context.Context) {
	tick := time.NewTicker(orDuration(c.Interval, ChangeInterval))
	defer tick.Stop()
	for {
		if err := c.Tick(ctx); err != nil && ctx.Err() == nil {
			c.log().Warn("library changes: tick failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Tick reads the feed once Every has passed since it was last read through. The first tick only
// starts the cursor: what changed before Loomarr followed the feed is the refresh's to catch.
func (c *ChangeFollower) Tick(ctx context.Context) error {
	if c.Enabled != nil && !c.Enabled() {
		return nil
	}
	now := c.now()
	last, ok, err := c.Store.LibraryCatalogFeedRead(ctx, c.Catalog)
	switch {
	case err != nil:
		return err
	case !ok:
		return c.Store.SaveLibraryCatalogFeedRead(ctx, c.Catalog, now)
	case now.Sub(last) < orDuration(c.Every, ChangeEvery):
		return nil
	}
	var refs []mp.Ref
	if now.Sub(last) <= c.Feed.Window() {
		refs, err = c.Feed.Changes(ctx, last)
	} else {
		err = mp.ErrUnsupported
	}
	switch {
	case err == nil:
	case errors.Is(err, mp.ErrUnsupported):
		// The feed no longer reaches back to the last read (Loomarr was off for longer than its
		// window), so what changed in between is unknown. The refresh, which fetches every record
		// within its period spread over its ticks, catches it; the feed is followed from now.
		c.log().Info("library changes: the feed no longer reaches the last read; the refresh catches up",
			"catalog", c.Catalog, "last_read", last)
		return c.Store.SaveLibraryCatalogFeedRead(ctx, c.Catalog, now)
	case errors.Is(err, mp.ErrNotConfigured), errors.Is(err, tmdb.ErrAPIKeyRequired):
		return nil
	default:
		// Failed: the cursor stays, so the next tick asks for the same span again.
		return err
	}
	keys := make([]librarystore.LibraryCatalogKey, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, librarystore.LibraryCatalogKey{Provider: string(ref.ID.Namespace), Kind: string(ref.Kind), ExternalID: ref.ID.Value})
	}
	marked, err := c.Store.MarkLibraryCatalogChanged(ctx, keys)
	if err != nil {
		return err
	}
	c.log().Debug("library changes: read the feed", "catalog", c.Catalog, "changed", len(refs), "held", marked)
	return c.Store.SaveLibraryCatalogFeedRead(ctx, c.Catalog, now)
}

func (c *ChangeFollower) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *ChangeFollower) log() *slog.Logger {
	if c.Log != nil {
		return c.Log
	}
	return slog.Default()
}
