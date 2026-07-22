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

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// GuidePoker nudges the media server after a channel-affecting reconcile so
// changes appear promptly (§9). Two operations (they are NOT interchangeable —
// see §9): PokeGuideRefresh updates EPG for KNOWN channels (an existing channel's
// lineup changed); RescanTuner re-reads the M3U channel LIST to discover a
// newly-created or removed channel. Best-effort: a failure degrades freshness,
// never the reconcile. Nil disables poking (e.g. Live TV not wired yet).
type GuidePoker interface {
	PokeGuideRefresh(ctx context.Context) error
	RescanTuner(ctx context.Context) error
}

// Availability resolves a provisioning Key to (libraryItemID, available). The
// engine backs it with the store's title records (a title is available iff its
// Record is in state `available`, carrying LibraryID). It satisfies
// schedule.Availability so the pure domain can consume it directly.
type Availability interface {
	schedule.Availability
}

// PodFiller assembles a channel's matched filler-clip pool (§10). Implemented by
// the filler package (a catalog-backed pod assembler); nil = flex-only (no pods).
// Called during reconcile after the channel exists: it returns the Tunarr program
// uuids of the matched clips, which the engine hands to the Programmer to build +
// attach the channel's Tunarr filler-list. Tunarr then plays those clips into the
// flex gaps the scheduler leaves between programs (§9 break interleave) — so this
// is a per-channel POOL, not a per-gap sequence. Seed-deterministic so the pool
// reproduces across reconciles (§10/§19), which is what makes the filler-list
// attach idempotent.
type PodFiller interface {
	// BuildFillerList returns the Tunarr program uuids of the matched clip pool for a
	// channel, given a deterministic seed and the channel's per-channel filler
	// Selection (§10 — era/audience/category/kinds + pinned/excluded). ok=false → no
	// pool (empty catalog / only the fallback card): the engine skips the attach and
	// the channel's flex falls back to the bumper card (never dead air).
	BuildFillerList(ctx context.Context, channelID string, seed int64, sel filler.Selection) (programIDs []string, ok bool)
}

// Engine reconciles channels against Tunarr. One per process; the per-channel
// mutex map serializes reconciles of the *same* channel (§18) while allowing
// different channels to reconcile concurrently.
type Engine struct {
	store store.Store
	prog  programmer.Programmer
	avail Availability
	guide GuidePoker
	pods  PodFiller // §10 pod assembly; nil = flex-only (Phase-10 default)
	// ratings heals an approved lineup entry that reached the scheduler UNRATED
	// (an acquisition that hadn't landed at proposal time, or a pre-fix cached
	// proposal — §389). Optional: nil ⇒ no heal (the entry stays unrated and a
	// fail-closed audience gate drops it). Bounded to unrated entries, so a
	// normally-stamped channel never calls it.
	ratings RatingResolver
	// notify publishes a UI-facing `channel` SSE frame after a reconcile changes a
	// channel, so the Channels/detail pages update live without a manual refresh
	// (§9 self-maintaining; the "no rebuild button" model). Optional: nil ⇒ no emit
	// (unit tests, no-events path). A local interface so this package needn't import
	// internal/events — same accept-interfaces style as GuidePoker/RatingResolver.
	notify ChannelNotifier
	log    *slog.Logger

	policy        schedule.PendingPolicy
	reconcileTTL  time.Duration // how far ahead to set a channel's next sweep deadline
	breaksPerHour int           // §10 commercial-break density applied per channel
	now           func() time.Time

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
	// BreaksPerHour is the commercial-break density (§10, FILLER_BREAKS_PER_HOUR)
	// applied to every channel at reconcile time. 0 = no breaks.
	BreaksPerHour int
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
		store:         st,
		prog:          prog,
		avail:         avail,
		guide:         guide,
		log:           log,
		policy:        cfg.Policy,
		reconcileTTL:  cfg.ReconcileTTL,
		breaksPerHour: cfg.BreaksPerHour,
		now:           now,
		locks:         map[string]*sync.Mutex{},
	}
}

// RatingResolver returns the content rating for an approved lineup entry, looked up
// by its Key against the library. ok=false when the title isn't present yet (so
// there's nothing to heal — it stays unrated until it lands). Implemented by a
// composition-root adapter over library.Client.LookupDetail.
type RatingResolver interface {
	Rating(ctx context.Context, key provision.Key) (rating string, ok bool, err error)
}

// WithRatings wires the heal-unrated-entries resolver (§389 amendment). Returns the
// engine for chaining; keeps New's signature stable.
func (e *Engine) WithRatings(r RatingResolver) *Engine {
	e.ratings = r
	return e
}

// WithPods enables ad-pod assembly for filler gaps (§10). Without it the engine
// leaves filler slots as flex (the Phase-10 default). Returns the engine for
// chaining. Kept as a setter so New's signature stays stable for Phase-10 callers.
func (e *Engine) WithPods(p PodFiller) *Engine {
	e.pods = p
	return e
}

// ChannelNotifier publishes a UI-facing "channel changed" signal (an SSE `channel`
// frame). A local interface so the channels package needn't import internal/events;
// the composition root adapts the event bus. `status` is the channel's Loomarr-side
// status after the reconcile, so a subscriber can update without a refetch if it wants.
type ChannelNotifier interface {
	ChannelChanged(channelID, status string)
}

// WithNotifier wires the `channel` SSE emitter so a reconcile that changes a channel
// updates the UI live (the "no manual rebuild" model, §9). Optional; nil ⇒ no emit.
func (e *Engine) WithNotifier(n ChannelNotifier) *Engine {
	e.notify = n
	return e
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
	store    store.Store
	ctx      context.Context
	duration DurationResolver // optional; nil ⇒ duration unknown (0), caller falls back
	episodes EpisodeResolver  // optional; nil ⇒ series resolve to a pending slot
}

// DurationResolver returns a library item's runtime in ms (§9/§10, from the media
// server's RunTimeTicks). Injected so the scheduler stays decoupled from the
// library client and unit tests need no live server.
type DurationResolver func(ctx context.Context, libraryItemID string) (int64, error)

// EpisodeResolver enumerates a series' episodes as playable programs (§9 series
// expansion), given the show's library item id. Injected (from the library
// adapter) so the scheduler stays decoupled and tests need no live server.
type EpisodeResolver func(ctx context.Context, showItemID string) ([]schedule.ResolvedProgram, error)

// NewStoreAvailability builds an Availability over the store's title records.
// The ctx bounds the lookups (they run inside a reconcile's context). dur/eps may
// be nil (e.g. tests): a nil dur ⇒ movies resolve with duration 0 (caller falls
// back to the entry's); a nil eps ⇒ series resolve to a pending slot.
func NewStoreAvailability(ctx context.Context, st store.Store, dur DurationResolver, eps EpisodeResolver) Availability {
	return &storeAvailability{store: st, ctx: ctx, duration: dur, episodes: eps}
}

func (s *storeAvailability) Resolve(key provision.Key) (string, int64, bool) {
	// A series isn't directly playable — it resolves via ResolveEpisodes.
	if key.IsSeries() {
		return "", 0, false
	}
	rec, err := s.store.GetTitle(s.ctx, key)
	if err != nil {
		return "", 0, false // not found / error → treat as unavailable
	}
	if rec.State != provision.Available || rec.LibraryID == "" {
		return "", 0, false
	}
	var durationMs int64
	if s.duration != nil {
		// Best-effort: a duration lookup failure shouldn't make an available title
		// vanish. ComputeDesired falls back to the entry's own duration on 0.
		if d, derr := s.duration(s.ctx, rec.LibraryID); derr == nil {
			durationMs = d
		}
	}
	return rec.LibraryID, durationMs, true
}

// ResolveEpisodes expands an available series into its episode programs (§9). The
// series' title Record carries the show's library id; the episode resolver
// enumerates the show's episodes from the library. Returns (nil, false) for a
// non-series, an unavailable series, or when no episode resolver is wired — the
// series then falls back to a pending slot upstream.
func (s *storeAvailability) ResolveEpisodes(key provision.Key) ([]schedule.ResolvedProgram, bool) {
	if !key.IsSeries() || s.episodes == nil {
		return nil, false
	}
	rec, err := s.store.GetTitle(s.ctx, key)
	if err != nil || rec.State != provision.Available || rec.LibraryID == "" {
		return nil, false
	}
	eps, err := s.episodes(s.ctx, rec.LibraryID)
	if err != nil || len(eps) == 0 {
		return nil, false
	}
	return eps, true
}
