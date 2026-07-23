package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
)

// LibraryScan is the poll-based availability path (design §4, §18.1) — the PRIMARY way a
// requested title reaches `available`, mirroring how Overseerr/Seerr work (poll the library,
// don't wait on a webhook). Where the reconciler's giveUp does a per-title Lookup only for
// deadline-due records, the scan lists what the media server recently added in ONE call and
// confirms every in-flight title now present. Same LibraryConfirmed → available transition,
// but continuous and not deadline-gated, so availability lands promptly without the inbound
// webhook (which is retired once this is proven).
type LibraryScan struct {
	store   store.Store
	scanner library.LibraryScanner
	emit    Emitter
	now     func() time.Time
	log     *slog.Logger

	// lookback bounds the incremental RecentlyAdded window. It intentionally exceeds the scan
	// interval (a few multiples) so a briefly-missed tick — a slow scan, a restart — still
	// re-observes recent imports rather than dropping them into the daily full-sweep gap.
	lookback time.Duration
}

// NewLibraryScan builds a scan. now defaults to time.Now; lookback defaults to 1h (comfortably
// wider than the default 5-minute scan cadence).
func NewLibraryScan(st store.Store, scanner library.LibraryScanner, emit Emitter, lookback time.Duration, now func() time.Time, log *slog.Logger) *LibraryScan {
	if now == nil {
		now = time.Now
	}
	if lookback <= 0 {
		lookback = time.Hour
	}
	return &LibraryScan{store: st, scanner: scanner, emit: emit, now: now, lookback: lookback, log: log}
}

// Incremental confirms availability for in-flight titles added to the library within the
// lookback window — the frequent (5-minute) job. Returns the number of titles confirmed.
func (s *LibraryScan) Incremental(ctx context.Context) (int, error) {
	since := s.now().Add(-s.lookback)
	items, err := s.scanner.RecentlyAdded(ctx, since)
	if err != nil {
		return 0, err
	}
	return s.confirm(ctx, items)
}

// Full confirms availability against the ENTIRE library — the periodic safety net (daily) for
// anything the incremental window missed (Loomarr down across a scan, a late-attached provider
// id on an older item). Returns the number of titles confirmed.
func (s *LibraryScan) Full(ctx context.Context) (int, error) {
	items, err := s.scanner.AllItems(ctx)
	if err != nil {
		return 0, err
	}
	return s.confirm(ctx, items)
}

// confirm correlates scanned library items against the in-flight title set and applies
// LibraryConfirmed to any match. It indexes the (small) in-flight set by provision.Key, then
// probes it with each (potentially many) scanned item's key — the key parity guarantee (same
// Key from a Title, a webhook, or a scan item) makes the match exact. O(items) probes.
func (s *LibraryScan) confirm(ctx context.Context, items []library.SearchResult) (int, error) {
	inflight, err := s.inflightByKey(ctx)
	if err != nil {
		return 0, err
	}
	if len(inflight) == 0 {
		return 0, nil // nothing awaiting the library; skip the correlation entirely
	}

	now := s.now()
	confirmed := 0
	seen := make(map[provision.Key]bool, len(inflight)) // one confirm per title per scan
	for _, it := range items {
		key, ok := scanItemKey(it)
		if !ok {
			continue // no usable provider id → can't correlate
		}
		rec, awaiting := inflight[key]
		if !awaiting || seen[key] {
			continue
		}
		seen[key] = true

		next, emitted := provision.Apply(rec, provision.Event{Kind: provision.LibraryConfirmed, LibraryID: it.LibraryItemID}, now)
		s.persist(ctx, next, emitted)
		confirmed++
	}
	return confirmed, nil
}

// inflightByKey loads every title awaiting the library (requested + downloading) indexed by
// key. These are the only states a library confirmation can advance; wanted has no release yet
// and terminal states are frozen (provision invariant 1).
func (s *LibraryScan) inflightByKey(ctx context.Context) (map[provision.Key]provision.Record, error) {
	out := make(map[provision.Key]provision.Record)
	for _, st := range []provision.State{provision.Requested, provision.Downloading} {
		recs, err := s.store.ListTitlesByState(ctx, st)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			out[r.Key] = r
		}
	}
	return out, nil
}

// persist writes the confirmed record and fans its terminal events to the emitter — identical
// to reconcile's persist so both availability paths behave the same downstream.
func (s *LibraryScan) persist(ctx context.Context, rec provision.Record, emitted []provision.DomainEvent) {
	if err := s.store.UpsertTitle(ctx, rec); err != nil {
		s.log.Error("library-scan: persist", "key", rec.Key, "err", err)
		return
	}
	for _, ev := range emitted {
		s.log.Info("provision event", "key", ev.Key, "state", ev.State, "src", "library-scan")
		if s.emit != nil {
			s.emit.Emit(ctx, ev)
		}
	}
}

// scanItemKey builds the provision.Key for a scanned library item via the SAME Title.Key()
// path the store used to key the record — so a series keyed series:tvdb:<id> and a movie keyed
// movie:tmdb:<id> match byte-for-byte. Returns false when the item carries no usable id.
func scanItemKey(it library.SearchResult) (provision.Key, bool) {
	mt := provision.Movie
	if it.MediaType == library.Series {
		mt = provision.Series
	}
	key, err := provision.Title{MediaType: mt, TMDBID: it.TMDBID, TVDBID: it.TVDBID}.Key()
	if err != nil {
		return "", false
	}
	return key, true
}
