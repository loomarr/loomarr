// Package channels is the channel reconcile engine (design §9/§18): the conductor
// that turns a store.Channel's approved lineup + live availability into an
// actual, filled Tunarr channel and keeps it that way. It owns the per-channel
// mutex (§18), pulls *desired* from the pure schedule domain, diffs it against
// the Programmer adapter's *actual*, and applies the minimal Tunarr calls
// (desired-vs-actual, idempotent). The periodic sweep (Runner) re-derives every
// channel from the store so availability events are a latency optimization,
// never load-bearing (§9) — the sweep is what makes backfill crash-safe and
// multi-replica correct.
package channels

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// GuidePoker triggers the media server's guide-refresh task after a
// channel-affecting reconcile (§9 guide freshness). Best-effort: a failure
// degrades freshness, never the reconcile. Nil disables poking (e.g. Live TV not
// wired yet).
type GuidePoker interface {
	PokeGuideRefresh(ctx context.Context) error
}

// Availability resolves a provisioning Key to (libraryItemID, available). The
// engine backs it with the store's title records (a title is available iff its
// Record is in state `available`, carrying LibraryID). It satisfies
// schedule.Availability so the pure domain can consume it directly.
type Availability interface {
	schedule.Availability
}

// Engine reconciles channels against Tunarr. One per process; the per-channel
// mutex map serializes reconciles of the *same* channel (§18) while allowing
// different channels to reconcile concurrently.
type Engine struct {
	store store.Store
	prog  programmer.Programmer
	avail Availability
	guide GuidePoker
	log   *slog.Logger

	policy       schedule.PendingPolicy
	reconcileTTL time.Duration // how far ahead to set a channel's next sweep deadline
	now          func() time.Time

	mu    sync.Mutex
	locks map[string]*sync.Mutex // per-channel-id mutex (§18)
}

// Config parameterizes the engine (from §15).
type Config struct {
	// Policy is the pending-slot policy (§9); default pod-fill if empty.
	Policy schedule.PendingPolicy
	// ReconcileTTL is how far ahead each reconcile schedules the channel's next
	// sweep (CHANNEL_RECONCILE_EVERY-aligned). The Runner ticks at that cadence;
	// this is the per-row lease horizon so ClaimDueChannels re-offers the channel.
	ReconcileTTL time.Duration
}

// New builds an Engine. guide may be nil (no guide poke). now defaults to
// time.Now.
func New(st store.Store, prog programmer.Programmer, avail Availability, guide GuidePoker, cfg Config, now func() time.Time, log *slog.Logger) *Engine {
	if cfg.Policy == "" {
		cfg.Policy = schedule.PodFill
	}
	if cfg.ReconcileTTL <= 0 {
		cfg.ReconcileTTL = 10 * time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &Engine{
		store:        st,
		prog:         prog,
		avail:        avail,
		guide:        guide,
		log:          log,
		policy:       cfg.Policy,
		reconcileTTL: cfg.ReconcileTTL,
		now:          now,
		locks:        map[string]*sync.Mutex{},
	}
}

// lockFor returns the per-channel mutex, creating it on first use (§18).
func (e *Engine) lockFor(id string) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()
	m, ok := e.locks[id]
	if !ok {
		m = &sync.Mutex{}
		e.locks[id] = m
	}
	return m
}

// storeAvailability adapts the store's title records to schedule.Availability: a
// key is available iff its Record is in state `available`, and its LibraryID is
// the item to play. This is the concrete Availability the engine uses in
// production; tests can substitute a map.
type storeAvailability struct {
	store store.Store
	ctx   context.Context
}

// NewStoreAvailability builds an Availability over the store's title records.
// The ctx bounds the lookups (they run inside a reconcile's context).
func NewStoreAvailability(ctx context.Context, st store.Store) Availability {
	return &storeAvailability{store: st, ctx: ctx}
}

func (s *storeAvailability) Resolve(key provision.Key) (string, bool) {
	rec, err := s.store.GetTitle(s.ctx, key)
	if err != nil {
		return "", false // not found / error → treat as unavailable
	}
	if rec.State != provision.Available || rec.LibraryID == "" {
		return "", false
	}
	return rec.LibraryID, true
}
