package scan

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/library/tmdb"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Enricher defaults (decision 20).
const (
	// EnrichInterval is how often the Enricher looks for titles to fetch.
	EnrichInterval = time.Minute
	// EnrichRefresh is how old a record may grow before it is fetched again. Each tick refetches a
	// share of the cache, so the whole of it is refreshed once per period, spread out.
	EnrichRefresh = 90 * 24 * time.Hour
	// enrichNewBatch caps the titles fetched for the first time in one tick.
	enrichNewBatch = 200
	// enrichRetry holds a failed fetch back.
	enrichRetry = 6 * time.Hour
)

// catalogVersions is the shape of record each kind is read in now. Raising a kind's version when
// the scanner starts reading something a cached record lacks fetches every record of that kind
// again, ahead of the refresh, as if new; a kind not listed is version 0.
//
//	series 1: a series counts its other episode orderings (decision 53).
//	series 2: a series carries each season's poster.
//	movie 1, series 3: every studio or network, the tagline, countries and trailer (decision 46).
//	movie 2, series 4: the main cast and key crew, a series' creators (decision 49).
//	series 5: a series lists its other episode orders, not only counts them (decision 53).
var catalogVersions = map[string]int{string(mp.KindMovie): 2, string(mp.KindSeries): 5}

// EnrichStore is the cache the Enricher fills.
type EnrichStore interface {
	DueLibraryCatalog(ctx context.Context, due store.LibraryCatalogDue) ([]store.LibraryCatalogKey, error)
	CountLibraryCatalog(ctx context.Context) (int, error)
	SaveLibraryCatalog(ctx context.Context, key store.LibraryCatalogKey, record string, version int, language string, at time.Time) (bool, error)
	FailLibraryCatalog(ctx context.Context, key store.LibraryCatalogKey, retryAt time.Time) error
	RewriteLibraryCatalogFiles(ctx context.Context, key store.LibraryCatalogKey) ([]store.LibraryFilePath, error)
}

// Enricher fetches the catalog records the walk asked for (#2151, decision 20): titles new to the
// cache first, then a share of the oldest, so every record is refreshed within EnrichRefresh. A
// record that changed sends its titles' folders to the scanner, which writes them again with it.
type Enricher struct {
	Store EnrichStore
	// Provider is the catalog (TMDB). One that is not configured fetches nothing, and is asked
	// again next tick.
	Provider mp.Provider
	// Rewrite asks for a pass over folders under a root.
	Rewrite func(root string, dirs ...string)
	// Language is the language records are read in now ("en-US"); a record cached in another is
	// fetched again first (decision 21). Nil, or "", checks none.
	Language func() string
	// Enabled is the scanner's kill switch; nil is on. Off, nothing is fetched.
	Enabled  func() bool
	Interval time.Duration
	Refresh  time.Duration
	Now      func() time.Time
	Log      *slog.Logger
	// Sleep waits between fetches, per the provider's rate hint; nil sleeps on the clock.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Run fetches until ctx ends.
func (e *Enricher) Run(ctx context.Context) {
	tick := time.NewTicker(orDuration(e.Interval, EnrichInterval))
	defer tick.Stop()
	for {
		if err := e.Tick(ctx); err != nil && ctx.Err() == nil {
			e.log().Warn("library enrich: tick failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Tick fetches what is due now. It stops early when the catalog cannot be reached at all.
func (e *Enricher) Tick(ctx context.Context) error {
	if e.Enabled != nil && !e.Enabled() {
		return nil
	}
	now := e.now()
	total, err := e.Store.CountLibraryCatalog(ctx)
	if err != nil {
		return err
	}
	due, err := e.Store.DueLibraryCatalog(ctx, store.LibraryCatalogDue{
		Now: now, StaleBefore: now.Add(-orDuration(e.Refresh, EnrichRefresh)),
		New: enrichNewBatch, Stale: e.staleShare(total), Versions: catalogVersions, Language: e.language(),
	})
	if err != nil {
		return err
	}
	gap := e.Provider.Capabilities().Rate.MinInterval
	for i, key := range due {
		if i > 0 && gap > 0 {
			if err := e.sleep(ctx, gap); err != nil {
				return err
			}
		}
		outcome, err := e.fetch(ctx, key, now)
		if err != nil {
			return err
		}
		if outcome != fetchDone {
			return nil
		}
	}
	return nil
}

// staleShare is how many old records one tick refreshes: enough that the whole cache is refreshed
// once per Refresh, at least one.
func (e *Enricher) staleShare(total int) int {
	ticks := int(orDuration(e.Refresh, EnrichRefresh) / orDuration(e.Interval, EnrichInterval))
	if ticks < 1 {
		ticks = 1
	}
	return total/ticks + 1
}

// Update fetches one title's record now, for "Update details" (decision 20), and writes its titles
// again when it changed. ErrNoCatalog when the catalog is not configured; ErrCatalogUnreachable
// when it failed, and the record is then tried again later as any failed fetch is.
func (e *Enricher) Update(ctx context.Context, key store.LibraryCatalogKey) error {
	outcome, err := e.fetch(ctx, key, e.now())
	switch {
	case err != nil:
		return err
	case outcome == fetchUnconfigured:
		return ErrNoCatalog
	case outcome == fetchFailed:
		return ErrCatalogUnreachable
	}
	return nil
}

// ErrCatalogUnreachable is a catalog fetch that failed: the catalog is down, slow or refused.
var ErrCatalogUnreachable = errors.New("the catalog could not be reached")

// fetchOutcome is how one fetch went.
type fetchOutcome int

const (
	// fetchDone fetched the record, or learned the catalog has no such title.
	fetchDone fetchOutcome = iota
	// fetchUnconfigured is a catalog with no key: nothing can be fetched until it has one.
	fetchUnconfigured
	// fetchFailed is a catalog that failed: the next key would fail too.
	fetchFailed
)

// fetch fetches one key.
func (e *Enricher) fetch(ctx context.Context, key store.LibraryCatalogKey, now time.Time) (fetchOutcome, error) {
	ref := mp.Ref{Kind: mp.Kind(key.Kind), ID: mp.ExternalID{Namespace: mp.Namespace(key.Provider), Value: key.ExternalID}}
	rec, fetchErr := e.Provider.Fetch(ctx, ref)
	record := ""
	switch {
	case fetchErr == nil:
		raw, err := json.Marshal(rec)
		if err != nil {
			return fetchDone, err
		}
		record = string(raw)
	case errors.Is(fetchErr, mp.ErrNotFound), errors.Is(fetchErr, mp.ErrUnsupported):
		// The catalog has no such title: remember that, so it is not asked again until the
		// refresh.
	case errors.Is(fetchErr, tmdb.ErrAPIKeyRequired):
		// Not configured: nothing is fetched until it is, and no title is held back for it.
		return fetchUnconfigured, nil
	default:
		if ctx.Err() != nil {
			return fetchFailed, ctx.Err()
		}
		if err := e.Store.FailLibraryCatalog(ctx, key, now.Add(enrichRetry)); err != nil {
			return fetchFailed, err
		}
		e.log().Debug("library enrich: fetch failed", "catalog", key.String(), "err", fetchErr)
		return fetchFailed, nil
	}
	// A record says the language it was read in; "the catalog has no such title" holds in any.
	language := rec.Language
	if fetchErr != nil {
		language = e.language()
	}
	changed, err := e.Store.SaveLibraryCatalog(ctx, key, record, catalogVersions[key.Kind], language, now)
	if err != nil || !changed {
		return fetchDone, err
	}
	files, err := e.Store.RewriteLibraryCatalogFiles(ctx, key)
	if err != nil {
		return fetchDone, err
	}
	rewriteFolders(files, e.Rewrite)
	return fetchDone, nil
}

func (e *Enricher) sleep(ctx context.Context, d time.Duration) error {
	return sleepFor(ctx, e.Sleep, d)
}

// sleepFor waits d with sleep, or on the clock when sleep is nil; it ends early with ctx.
func sleepFor(ctx context.Context, sleep func(context.Context, time.Duration) error, d time.Duration) error {
	if sleep != nil {
		return sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (e *Enricher) language() string {
	if e.Language != nil {
		return e.Language()
	}
	return ""
}

func (e *Enricher) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Enricher) log() *slog.Logger {
	if e.Log != nil {
		return e.Log
	}
	return slog.Default()
}
