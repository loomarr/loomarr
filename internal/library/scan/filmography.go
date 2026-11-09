package scan

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/library/tmdb"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Filmographer defaults (decision 50).
const (
	// FilmographyInterval is how often the Filmographer looks for people to fetch.
	FilmographyInterval = time.Hour
	// FilmographyRefresh is how old a filmography may grow before it is fetched again.
	FilmographyRefresh = 30 * 24 * time.Hour
	// filmographyBatch caps the people fetched in one tick.
	filmographyBatch = 100
	// filmographyRetry holds a failed fetch back.
	filmographyRetry = 6 * time.Hour
)

// FilmographyCatalog is the catalog filmographies come from; its Capabilities.Catalog names the
// namespace it knows people by.
type FilmographyCatalog interface {
	Filmography(ctx context.Context, person mp.ExternalID) ([]mp.FilmCredit, error)
	Capabilities() mp.Capabilities
}

// Filmographer keeps the filmographies of the people the library credits (#2151 decision 50): a
// person's is fetched when they first qualify and monthly after. Each tick first removes the
// people no title credits any more.
type Filmographer struct {
	Store   librarystore.LibraryPeopleStore
	Catalog FilmographyCatalog
	// Enabled is the scanner's kill switch; nil is on. Off, nothing is fetched.
	Enabled  func() bool
	Interval time.Duration
	Refresh  time.Duration
	Now      func() time.Time
	Log      *slog.Logger
	// Sleep waits between fetches, per the catalog's rate hint; nil sleeps on the clock.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Run fetches until ctx ends.
func (f *Filmographer) Run(ctx context.Context) {
	tick := time.NewTicker(orDuration(f.Interval, FilmographyInterval))
	defer tick.Stop()
	for {
		if err := f.Tick(ctx); err != nil && ctx.Err() == nil {
			f.log().Warn("library filmographies: tick failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Tick prunes the people no title credits, then fetches the filmographies due now. It stops early
// when the catalog is not configured or fails.
func (f *Filmographer) Tick(ctx context.Context) error {
	if f.Enabled != nil && !f.Enabled() {
		return nil
	}
	if n, err := f.Store.PruneLibraryPeople(ctx); err != nil {
		return err
	} else if n > 0 {
		f.log().Debug("library filmographies: removed people no title credits", "count", n)
	}
	now := f.now()
	caps := f.Catalog.Capabilities()
	// Only people the catalog knows by its own ids have a filmography there.
	due, err := f.Store.DueLibraryFilmographies(ctx, librarystore.LibraryFilmographyDue{
		Namespace: string(caps.Catalog), Now: now, StaleBefore: now.Add(-orDuration(f.Refresh, FilmographyRefresh)),
		Limit: filmographyBatch,
	})
	if err != nil {
		return err
	}
	gap := caps.Rate.MinInterval
	for i, person := range due {
		if i > 0 && gap > 0 {
			if err := sleepFor(ctx, f.Sleep, gap); err != nil {
				return err
			}
		}
		credits, err := f.Catalog.Filmography(ctx, externalID(person.External))
		switch {
		case err == nil, errors.Is(err, mp.ErrNotFound), errors.Is(err, mp.ErrUnsupported):
			// None known is an answer too: it is not asked again until the refresh.
		case errors.Is(err, mp.ErrNotConfigured), errors.Is(err, tmdb.ErrAPIKeyRequired):
			return nil
		default:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			f.log().Debug("library filmographies: fetch failed", "person", person.ID, "err", err)
			return f.Store.FailLibraryFilmography(ctx, person.ID, now.Add(filmographyRetry))
		}
		entries := make([]librarystore.LibraryFilmEntry, 0, len(credits))
		for _, c := range credits {
			entries = append(entries, librarystore.LibraryFilmEntry{Kind: string(c.Kind), ID: libraryID(c.ID), Title: c.Title,
				Year: c.Year, Role: string(c.Role), Character: c.Character})
		}
		if err := f.Store.SaveLibraryFilmography(ctx, person.ID, entries, now); err != nil {
			return err
		}
	}
	return nil
}

func (f *Filmographer) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Filmographer) log() *slog.Logger {
	if f.Log != nil {
		return f.Log
	}
	return slog.Default()
}
