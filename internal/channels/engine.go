// Package channels is the channel reconcile engine (design §9/§18): the conductor
// that turns a store.Channel's approved lineup + live availability into durable
// desired state for whichever playout backend owns it. Tunarr-backed channels then
// project that state through Programmer with a minimal desired-vs-actual diff;
// internal-playout channels stop at the durable local result. The periodic sweep
// (Runner) re-derives every channel from the store so availability events are a
// latency optimization, never load-bearing (§9) — the sweep is what makes backfill
// crash-safe and multi-replica correct.
package channels

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
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

// EngineStore is the durable state the reconcile engine consumes. Keeping this
// interface with the consumer makes new persistence dependencies an explicit
// channels-package decision instead of inheriting the aggregate store surface.
type EngineStore interface {
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	GetChannel(ctx context.Context, id string) (channelstore.Channel, error)
	AttachTunarrChannel(ctx context.Context, id, expectedTunarrID, newTunarrID string, expectedNumber, newNumber int) (int64, error)
	DeleteChannel(ctx context.Context, id string, expectedRevision int64) error
	GetJob(ctx context.Context, id string) (proposalstore.Job, error)
	SaveChannel(ctx context.Context, ch channelstore.Channel) (channelstore.Channel, error)
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
	LastAiredByChannel(ctx context.Context, channelID string, before time.Time) (map[string]time.Time, error)
	RecordScheduledAirings(ctx context.Context, channelID string, airings []channeldatastore.ScheduledAiring, recordedAt time.Time) error
	LastScheduledByChannel(ctx context.Context, channelID string, before time.Time) (map[string]time.Time, error)
}

// AvailabilityStore is the title store used to resolve scheduled content. It is separate from
// EngineStore because availability is independently replaceable in tests and at composition time.
type AvailabilityStore interface {
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
}

// PodFiller answers the backend-specific views of a channel's matched filler
// pool (§10). Implemented by the filler package; nil = flex-only (no pods).
// Internal playout needs the playable duration because it resolves local clip paths per gap.
// Tunarr projection additionally needs BuildFillerList's program uuids to attach a
// remote filler-list. Both are seed-deterministic (§10/§19).
type PodFiller interface {
	// BuildFillerList returns the Tunarr program uuids of the matched clip pool for a
	// channel, given a deterministic seed and the channel's per-channel filler
	// Selection (§10 — era/audience/category/roles + pinned/excluded). ok=false → no
	// pool (empty catalog / only the fallback card): the engine skips the attach and
	// the channel's flex falls back to the bumper card (never dead air).
	BuildFillerList(ctx context.Context, channelID string, seed int64, sel pod.Selection) (programIDs []string, ok bool)
	// HasPool reports whether the channel has ANY playable commercials — the question
	// "should breaks be interleaved at all?".
	//
	// Separate from BuildFillerList (§9.1) because the two ask different questions.
	// BuildFillerList is Tunarr-specific: it returns only clips carrying a Tunarr program uuid,
	// since that is what a filler-list references. On an internal-playout install with no
	// Tunarr, no clip has one — so a channel with a perfectly good pod would report "no pool"
	// and get NO BREAKS AT ALL.
	HasPool(ctx context.Context, channelID string, seed int64, sel pod.Selection) bool
	// PlayableDurationMs reports how much real media the deterministic pod contains.
	// It excludes the generated fallback card, which has no bytes to air.
	PlayableDurationMs(ctx context.Context, channelID string, seed int64, sel pod.Selection) int64
}

// Engine reconciles every channel's local desired state and, only when that channel's
// effective backend is Tunarr, projects it through Programmer. One per process; the
// per-channel mutex map serializes reconciles of the *same* channel (§18) while
// allowing different channels to reconcile concurrently.
type Engine struct {
	store EngineStore
	prog  tunarr.Programmer
	avail Availability
	guide GuidePoker
	pods  PodFiller // §10 pod assembly; nil = flex-only (Phase-10 default)
	// ratings heals an approved lineup entry that reached the scheduler UNRATED
	// (an acquisition that hadn't landed at proposal time — §389). Optional: nil ⇒ no heal (the entry stays unrated and a
	// fail-closed audience gate drops it). Bounded to unrated entries, so a
	// normally-stamped channel never calls it.
	ratings RatingResolver
	// franchises heals a movie entry's TMDB collection id (§5 franchise ordering) so a
	// franchise's films stay together, in release order. Optional (nil ⇒ no grouping).
	franchises FranchiseResolver
	// boxSets stamps a title's media-server collection membership (§2.2) so scope.collections
	// enforces with no library I/O. Optional (nil ⇒ membership never resolves and the filter
	// admits everything, rather than emptying a lineup).
	boxSets BoxSetResolver
	// notify publishes a UI-facing `channel` SSE frame after a reconcile changes a
	// channel, so the Channels/detail pages update live without a manual refresh
	// (§9 self-maintaining; the "no rebuild button" model). Optional: nil ⇒ no emit
	// (unit tests, no-events path). A local interface so this package needn't import
	// internal/platform/events — same accept-interfaces style as GuidePoker/RatingResolver.
	notify ChannelNotifier
	// scheduleInvalidator cuts over process-local internal playout after a newly
	// committed Desired cycle differs from the one an active encoder may be using.
	// Optional: nil when internal playout is not wired.
	scheduleInvalidator ScheduleInvalidator
	// acts records operator-facing facts that must outlive a log line (§9 V54) — currently the
	// automatic renumber when Tunarr already occupies a channel's number. Optional/nil-safe.
	acts    ActivityRecorder
	metrics MetricsObserver
	quality SchedulingQuality
	log     *slog.Logger

	policy            schedule.PendingPolicy
	reconcileTTLFor   func() time.Duration                              // live minimum delay before the next sweep eligibility
	breaksPerHourFor  func() int                                        // live §10 commercial-break default
	breakDurationFor  func() time.Duration                              // live §10 commercial-break length default
	defaultWindowFor  func() time.Duration                              // live §6.5 rolling-window default
	calendarFor       func() schedule.Calendar                          // window grid's wall clock (#1675) and holidays (#2030)
	playoutBackendFor func(context.Context) (string, error)             // durable §9.1 transition target
	naturalBreaks     func(context.Context) schedule.NaturalBreakSource // §10 mid-roll candidates; nil = none
	premeasure        func(context.Context, []string)                   // #1518 measure-ahead; nil = none
	now               func() time.Time

	mu    sync.Mutex
	locks map[string]*sync.Mutex // per-channel-id mutex (§18)
}

// MetricsObserver receives bounded reconcile facts without Channel identity.
type MetricsObserver interface {
	ChannelReconciled(duration time.Duration, success bool)
	ChannelSlotSubstitutions(count int)
}

// SchedulingQuality receives only the Proposal Job identity and bounded attempt
// facts. Implementations own opaque keys, privacy, and best-effort persistence.
type SchedulingQuality interface {
	ProposalSchedulingFailed(context.Context, string, time.Time, time.Duration)
	ProposalScheduled(context.Context, string, time.Time, time.Duration)
}

// Config parameterizes the engine (from §15).
type Config struct {
	// Policy is the pending-slot policy (§9); default pod-fill if empty.
	Policy schedule.PendingPolicy
	// ReconcileTTL is how far ahead each reconcile schedules the channel's next
	// sweep (CHANNEL_RECONCILE_EVERY-aligned). The Runner ticks at that cadence;
	// this is the per-row lease horizon so ClaimDueChannels re-offers the channel.
	ReconcileTTL time.Duration
	// ResolveReconcileTTL reads a live settings value when provided. Tests and callers with
	// immutable configuration can leave it nil and use ReconcileTTL.
	ResolveReconcileTTL func() time.Duration
	// BreaksPerHour is the commercial-break density (§10, FILLER_BREAKS_PER_HOUR)
	// applied to every channel at reconcile time. 0 = no breaks.
	BreaksPerHour        int
	ResolveBreaksPerHour func() int
	BreakDuration        time.Duration
	ResolveBreakDuration func() time.Duration
	// DefaultWindow is the global rolling-window horizon (§6.5, sched.window_hours,
	// default 24h) — how far ahead each channel materializes before it rolls forward.
	// A per-channel/-rule Window overrides it; 0 = schedule the whole run.
	DefaultWindow        time.Duration
	ResolveDefaultWindow func() time.Duration
	// ResolveCalendar is the household's calendar: the wall clock the rolling-window grid is laid
	// on (guide.timezone, else the container's zone), so a daily window turns at local midnight
	// (#1675), and the holidays its country keeps (filler.home_country, #2030). Nil = the zero
	// Calendar: UTC, the US, the shipped definitions.
	ResolveCalendar func() schedule.Calendar
	// ResolvePlayoutBackendContext reads the durable transition checkpoint once per reconcile
	// attempt so Postgres replicas observe Prepared. Nil fails closed through the empty backend.
	ResolvePlayoutBackendContext func(context.Context) (string, error)
	// NaturalBreaks returns the measured scene-fade source mid-roll placement reads (§10), bound
	// to one reconcile or preview pass. Nil = no mid-roll anywhere (between-programme breaks only).
	NaturalBreaks func(context.Context) schedule.NaturalBreakSource
	// Premeasure queues the library items an internal channel airs within PremeasureHorizon for
	// the one low-priority measure worker (#1518). It must not block. Nil = measure at first play.
	Premeasure func(ctx context.Context, libraryItemIDs []string)
}

// New builds an Engine. guide may be nil (no guide poke). now defaults to
// time.Now.
func New(st EngineStore, prog tunarr.Programmer, avail Availability, guide GuidePoker, cfg Config, now func() time.Time, log *slog.Logger) *Engine {
	if cfg.Policy == "" {
		cfg.Policy = schedule.PodFill
	}
	if cfg.ReconcileTTL <= 0 {
		cfg.ReconcileTTL = 10 * time.Minute
	}
	if cfg.ResolveReconcileTTL == nil {
		cfg.ResolveReconcileTTL = func() time.Duration { return cfg.ReconcileTTL }
	}
	if cfg.ResolveBreaksPerHour == nil {
		cfg.ResolveBreaksPerHour = func() int { return cfg.BreaksPerHour }
	}
	if cfg.ResolveBreakDuration == nil {
		cfg.ResolveBreakDuration = func() time.Duration { return cfg.BreakDuration }
	}
	if cfg.ResolveDefaultWindow == nil {
		cfg.ResolveDefaultWindow = func() time.Duration { return cfg.DefaultWindow }
	}
	if cfg.ResolveCalendar == nil {
		cfg.ResolveCalendar = func() schedule.Calendar { return schedule.Calendar{} }
	}
	if cfg.ResolvePlayoutBackendContext == nil {
		cfg.ResolvePlayoutBackendContext = func(context.Context) (string, error) { return "", nil }
	}
	if now == nil {
		now = time.Now
	}
	return &Engine{
		store:             st,
		prog:              prog,
		avail:             avail,
		guide:             guide,
		log:               log,
		policy:            cfg.Policy,
		reconcileTTLFor:   cfg.ResolveReconcileTTL,
		breaksPerHourFor:  cfg.ResolveBreaksPerHour,
		breakDurationFor:  cfg.ResolveBreakDuration,
		defaultWindowFor:  cfg.ResolveDefaultWindow,
		calendarFor:       cfg.ResolveCalendar,
		playoutBackendFor: cfg.ResolvePlayoutBackendContext,
		naturalBreaks:     cfg.NaturalBreaks,
		premeasure:        cfg.Premeasure,
		now:               now,
		locks:             map[string]*sync.Mutex{},
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

// FranchiseResolver returns the TMDB collection (franchise) id a MOVIE belongs to (§5
// franchise ordering), so the scheduler can keep a franchise's films together in release
// order. id 0 = standalone (no collection) — a legitimate resolved answer, distinct from
// "not yet resolved". ok=false ⇒ couldn't resolve (no TMDB / not a movie) — leave it to try
// next pass. Implemented by a composition-root adapter over the tmdb.Client.
type FranchiseResolver interface {
	Collection(ctx context.Context, key provision.Key) (collectionID int, ok bool, err error)
}

// WithFranchises wires the resolver that heals a movie entry's franchise CollectionID at
// reconcile (§5). Optional (nil ⇒ no franchise grouping). Returns the engine for chaining.
func (e *Engine) WithFranchises(r FranchiseResolver) *Engine {
	e.franchises = r
	return e
}

// BoxSetResolver returns the household collections (library_collections) a title belongs to,
// backing scope.collections (programming-design §2.2). Every reconcile asks it for every entry,
// so it must answer from memory. An empty slice with ok=true is a real answer, "belongs to no
// collection"; ok=false means it could not be resolved, and the entry keeps its last stamp.
//
// ⚠ Nothing to do with FranchiseResolver above, which resolves the TMDB franchise id.
// Same word, different namespaces (programming-design §2.2).
type BoxSetResolver interface {
	BoxSets(ctx context.Context, key provision.Key) (boxSetIDs []string, ok bool, err error)
}

// WithBoxSets wires the resolver that stamps a title's media-server collection membership
// at reconcile (§2.2). Optional (nil ⇒ membership never resolves, and scope.collections
// consequently admits everything rather than emptying a lineup — see boxSetOK's fail-open
// rule). Returns the engine for chaining.
func (e *Engine) WithBoxSets(r BoxSetResolver) *Engine {
	e.boxSets = r
	return e
}

// WithPods enables ad-pod assembly for filler gaps (§10). Without it the engine
// leaves filler slots as flex. Returns the engine for chaining.
func (e *Engine) WithPods(p PodFiller) *Engine {
	e.pods = p
	return e
}

// ChannelNotifier publishes a UI-facing "channel changed" signal (an SSE `channel`
// frame). A local interface so the channels package needn't import internal/platform/events;
// the composition root adapts the event bus. The status pair lets downstream product
// notifications distinguish a real transition from an ordinary reconciliation while
// UI subscribers still receive every committed change.
type ChannelNotifier interface {
	ChannelChanged(channelID, previousStatus, status string)
}

// ScheduleInvalidator retires an internal-playout session whose durable cycle
// changed. The next viewer request starts a session from the committed cycle.
type ScheduleInvalidator interface {
	StopChannel(channelID string)
}

// WithScheduleInvalidator wires internal playout's process-local session manager.
// Postgres durable invalidations perform the same cutover on peer replicas.
func (e *Engine) WithScheduleInvalidator(invalidator ScheduleInvalidator) *Engine {
	e.scheduleInvalidator = invalidator
	return e
}

// WithNotifier wires the `channel` SSE emitter so a reconcile that changes a channel
// updates the UI live (the "no manual rebuild" model, §9). Optional; nil ⇒ no emit.
func (e *Engine) WithNotifier(n ChannelNotifier) *Engine {
	e.notify = n
	return e
}

// ActivityRecorder is the durable operator-facing feed (§12). A local interface so this package
// needn't import internal/platform/activity — the same accept-interfaces style as ChannelNotifier above.
type ActivityRecorder interface {
	Warn(ctx context.Context, kind, subjectID, text string)
}

// WithActivity wires the durable feed. Optional; nil ⇒ the log line is the only record, which is
// exactly the gap that made a stranded channel take source-reading to diagnose (§9 V54).
func (e *Engine) WithActivity(a ActivityRecorder) *Engine {
	e.acts = a
	return e
}

// WithMetrics binds one application generation's bounded reconcile observer.
func (e *Engine) WithMetrics(observer MetricsObserver) *Engine {
	e.metrics = observer
	return e
}

// WithQualityRecorder wires the first-live Proposal scheduling journey.
func (e *Engine) WithQualityRecorder(recorder SchedulingQuality) *Engine {
	e.quality = recorder
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
	store    AvailabilityStore
	ctx      context.Context
	duration DurationResolver // optional; nil ⇒ duration unknown (0), caller falls back
	// durations is the BULK form of `duration`, used to prewarm durMemo before a layout so the
	// per-key Resolve calls all hit the memo instead of issuing one HTTP request each. Optional:
	// nil ⇒ no prewarm; every key takes the per-item path.
	durations DurationsResolver
	episodes  EpisodeResolver // optional; nil ⇒ series resolve to a pending slot

	// Memo over the three lookups a cycle layout repeats. ComputeDesiredAt walks the lineup
	// several times while laying out a cycle, and without the memo each miss is a fresh
	// round-trip to the media server (one guide request issued 72 library calls).
	//
	// ⚠ LIFETIME. The composition root builds ONE of these at boot with the process-lifetime
	// `rootCtx` (app.go), so this memo lives for the life of the process — it is NOT
	// request-scoped, whatever the call sites look like. That is why every entry carries an
	// expiry: without one, a title that finished acquiring would read `pending` forever, and a
	// re-encoded item would keep its old duration until a restart.
	//
	// The TTL is deliberately SHORT. It exists to collapse the repeats inside one operation
	// (the thing that cost 72 calls), not to serve as a real cache — the durable answer
	// for episode enumeration is the Library's own tables. Set it longer and this quietly
	// becomes a cache with no invalidation signal.
	//
	// Guarded by a mutex: the guide resolves channels concurrently, and a map is not safe
	// under that on its own.
	mu        sync.Mutex
	titleMemo map[provision.Key]titleLookup
	durMemo   map[string]memoEntry[int64]
	epsMemo   map[string]memoEntry[episodeLookup]
	now       func() time.Time
}

// memoTTL bounds every memoized answer. Long enough to collapse the repeated resolves within
// one cycle layout (milliseconds apart), short enough that no stale answer survives a reconcile
// interval — availability changes as acquisitions land, and the scheduler must see that.
const memoTTL = 5 * time.Second

// memoEntry is a memoized value with its expiry. Generic so the duration and episode caches
// share one expiry rule rather than each inventing its own.
type memoEntry[T any] struct {
	val T
	exp time.Time
}

// titleLookup memoizes one GetTitle result, including the MISS — re-asking the store for a key
// that was absent a millisecond ago costs the same as asking for one that was present.
// `exp` bounds it: see the lifetime note on storeAvailability.
type titleLookup struct {
	exp time.Time
	rec provision.Record
	err error
}

// DurationResolver returns a library item's runtime in ms (§9/§10, from the media
// server's RunTimeTicks). Injected so the scheduler stays decoupled from the
// library client and unit tests need no live server.
type DurationResolver func(ctx context.Context, libraryItemID string) (int64, error)

// DurationsResolver is DurationResolver in BULK: many item ids, one round trip.
//
// It exists because the per-item resolver is an N+1 the scheduler cannot batch on its own.
// ComputeDesiredAt asks Resolve(key) one key at a time as it lays out the cycle, so with only
// the single-item resolver a 25-movie channel issues 25 sequential HTTP calls to the media
// server. Measured against the dev Emby (behind Cloudflare, ~15ms per round trip): 25 calls ≈
// 375ms, which was the whole of GET /v1/guide's cold latency — the arrangement itself is
// microseconds.
//
// The media server answers this in one call: library.ItemMetadataByID requests RunTimeTicks and
// returns RuntimeMs for a comma-separated id list ("120 episodes returned in 24ms").
//
// Missing ids may be absent from the result — the caller treats an absent id as an unknown
// duration (0) exactly as a failed single lookup does.
type DurationsResolver func(ctx context.Context, libraryItemIDs []string) (map[string]int64, error)

// EpisodeResolver enumerates a series' episodes as playable programs (§9 series
// expansion), given the show's library item id. Injected (from the library
// adapter) so the scheduler stays decoupled and tests need no live server.
type EpisodeResolver func(ctx context.Context, showItemID string) ([]schedule.ResolvedProgram, error)

// NewStoreAvailability builds an Availability over the store's title records.
// The ctx bounds the lookups made through the Availability itself (the guide and the
// previews); a reconcile reads through its own view, under its own context. dur/eps may
// be nil (e.g. tests): a nil dur ⇒ movies resolve with duration 0 (caller falls
// back to the entry's); a nil eps ⇒ series resolve to a pending slot.
func NewStoreAvailability(ctx context.Context, st AvailabilityStore, dur DurationResolver, eps EpisodeResolver) Availability {
	return &storeAvailability{
		store: st, ctx: ctx, duration: dur, episodes: eps,
		titleMemo: map[provision.Key]titleLookup{},
		durMemo:   map[string]memoEntry[int64]{},
		epsMemo:   map[string]memoEntry[episodeLookup]{},
		now:       time.Now,
	}
}

// WithBulkDurations attaches the bulk duration resolver used to prewarm the duration memo
// (see DurationsResolver / PrewarmDurations), and returns the same Availability for chaining.
//
// A SETTER rather than a NewStoreAvailability parameter on purpose: callers that do not use the
// capability need not pass a `nil`, and an install that never calls this simply skips the prewarm.
//
// Takes an Availability (not the unexported concrete type) so the composition root can call it
// without reaching inside this package; a non-storeAvailability implementation is returned
// untouched, which is the correct no-op for the test doubles that substitute plain maps.
func WithBulkDurations(a Availability, d DurationsResolver) Availability {
	if sa, ok := a.(*storeAvailability); ok {
		sa.durations = d
	}
	return a
}

// clock reads the injected time source. A field rather than a direct time.Now call so the memo's
// expiry is testable without sleeping.
func (s *storeAvailability) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// title reads a title record, memoized for memoTTL. Short-lived on purpose: a title's State
// changes as an acquisition lands, and a permanent memo would leave a channel reading `pending`
// long after the title became available.
func (s *storeAvailability) title(ctx context.Context, key provision.Key) (provision.Record, error) {
	now := s.clock()
	s.mu.Lock()
	if hit, ok := s.titleMemo[key]; ok && now.Before(hit.exp) {
		s.mu.Unlock()
		return hit.rec, hit.err
	}
	s.mu.Unlock()

	// Deliberately OUTSIDE the lock: a store read can block, and holding the mutex across it
	// would serialize every channel the guide resolves concurrently — trading one bottleneck
	// for another. A duplicate read on a race is cheap and harmless; both writers store the
	// same answer.
	rec, err := s.store.GetTitle(ctx, key)

	s.mu.Lock()
	s.titleMemo[key] = titleLookup{rec: rec, err: err, exp: now.Add(memoTTL)}
	s.mu.Unlock()
	return rec, err
}

// itemDuration resolves a library item's runtime, memoized for memoTTL.
// A failure memoizes as 0 — the caller falls back to the lineup entry's own duration, and
// retrying a failing lookup once per occurrence is what made this slow in the first place.
func (s *storeAvailability) itemDuration(ctx context.Context, libraryID string) int64 {
	if s.duration == nil {
		return 0
	}
	now := s.clock()
	s.mu.Lock()
	if d, ok := s.durMemo[libraryID]; ok && now.Before(d.exp) {
		s.mu.Unlock()
		return d.val
	}
	s.mu.Unlock()

	var ms int64
	if d, err := s.duration(ctx, libraryID); err == nil {
		ms = d
	}

	s.mu.Lock()
	s.durMemo[libraryID] = memoEntry[int64]{val: ms, exp: now.Add(memoTTL)}
	s.mu.Unlock()
	return ms
}

// PrewarmDurations resolves many item durations in ONE round trip and seeds durMemo, so the
// per-key Resolve calls that follow all hit the memo.
//
// This is the fix for the N+1 described on DurationsResolver: without it a 25-movie channel
// issues 25 sequential media-server calls during a single layout, which measured as ~375ms of
// GET /v1/guide's cold latency.
//
// BEST-EFFORT, and deliberately so. A bulk failure seeds nothing and leaves every key to the
// existing per-item path — slower, but identical in outcome. It must never make a layout fail:
// duration is a refinement (ComputeDesired falls back to the lineup entry's own duration), not a
// precondition.
//
// Ids ALREADY MEMOIZED are not re-requested: a second channel sharing a title with the first
// should not re-fetch it, which is the same reason ItemMetadataByID de-duplicates its input.
// Entries are written with the same memoTTL expiry the per-item path uses, so this changes WHEN
// the answer is fetched, never how long it is trusted — the lifetime warning on storeAvailability
// still holds in full.
func (s *storeAvailability) PrewarmDurations(libraryIDs []string) {
	if s.durations == nil || len(libraryIDs) == 0 {
		return
	}
	now := s.clock()

	// Only ask for what is actually missing or expired.
	s.mu.Lock()
	want := make([]string, 0, len(libraryIDs))
	seen := make(map[string]bool, len(libraryIDs))
	for _, id := range libraryIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if d, ok := s.durMemo[id]; ok && now.Before(d.exp) {
			continue
		}
		want = append(want, id)
	}
	s.mu.Unlock()
	if len(want) == 0 {
		return
	}

	got, err := s.durations(s.ctx, want)
	if err != nil && len(got) == 0 {
		return // nothing usable; the per-item path still covers every key
	}

	// Seed MISSES as 0 too, exactly as itemDuration memoizes a failed lookup. Without this an id
	// the server did not return would fall through to a per-item call for every occurrence —
	// re-introducing the N+1 for precisely the items the bulk call could not answer.
	s.mu.Lock()
	exp := now.Add(memoTTL)
	for _, id := range want {
		s.durMemo[id] = memoEntry[int64]{val: got[id], exp: exp}
	}
	s.mu.Unlock()
}

func (s *storeAvailability) Resolve(key provision.Key) (string, int64, bool) {
	id, durationMs, available, _ := s.resolve(s.ctx, key)
	return id, durationMs, available
}

// resolve is Resolve with the one thing the scheduler's seam cannot carry: whether the answer
// is known. A title with no record is not in the library, which is an answer. Any other failed
// read is returned as an error beside "unavailable", so a caller that is about to write what it
// learns (the reconcile, through availabilityRun) can refuse to.
func (s *storeAvailability) resolve(ctx context.Context, key provision.Key) (string, int64, bool, error) {
	// A series isn't directly playable — it resolves via ResolveEpisodes.
	if key.IsSeries() {
		return "", 0, false, nil
	}
	rec, err := s.title(ctx, key)
	if err != nil {
		return "", 0, false, readFailure(key, err)
	}
	if rec.State != provision.Available || rec.LibraryID == "" {
		return "", 0, false, nil
	}
	// Best-effort: a duration lookup failure shouldn't make an available title vanish.
	// ComputeDesired falls back to the entry's own duration on 0.
	return rec.LibraryID, s.itemDuration(ctx, rec.LibraryID), true, nil
}

// readFailure is nil when the store answered "no such title", and the wrapped error otherwise.
func readFailure(key provision.Key, err error) error {
	if isNotFound(err) {
		return nil
	}
	return fmt.Errorf("read title %s: %w", key, err)
}

// ResolveEpisodes expands an available series into its episode programs (§9). The
// series' title Record carries the show's library id; the episode resolver
// enumerates the show's episodes from the library. Returns (nil, false) for a
// non-series, an unavailable series, or when no episode resolver is wired — the
// series then falls back to a pending slot upstream.
func (s *storeAvailability) ResolveEpisodes(key provision.Key) schedule.EpisodeResolution {
	resolution, _ := s.resolveEpisodes(s.ctx, key)
	return resolution
}

// resolveEpisodes is ResolveEpisodes with whether the answer is known, as resolve is for
// Resolve: a failed title read or a failed episode read comes back as an error beside the
// empty resolution.
func (s *storeAvailability) resolveEpisodes(ctx context.Context, key provision.Key) (schedule.EpisodeResolution, error) {
	if !key.IsSeries() || s.episodes == nil {
		return schedule.EpisodeResolution{}, nil
	}
	rec, err := s.title(ctx, key)
	if err != nil {
		return schedule.EpisodeResolution{}, readFailure(key, err)
	}
	if rec.State != provision.Available || rec.LibraryID == "" {
		return schedule.EpisodeResolution{}, nil
	}

	// A series in the lineup is re-resolved on every layout pass, so the memo collapses the
	// repeats within one layout. Behind it the Library's tables answer (#2151 step 16): its
	// largest show reads in under 10ms, so no persisted cache sits between them.
	now := s.clock()
	s.mu.Lock()
	if hit, ok := s.epsMemo[rec.LibraryID]; ok && now.Before(hit.exp) {
		s.mu.Unlock()
		return hit.val.resolution, hit.val.err
	}
	s.mu.Unlock()

	var lookup episodeLookup
	eps, err := s.episodes(ctx, rec.LibraryID)
	if err != nil {
		lookup.err = fmt.Errorf("read the episodes of %s: %w", key, err)
	} else {
		lookup.resolution.Programs = eps
	}
	s.memoEpisodes(rec.LibraryID, lookup, now)
	return lookup.resolution, lookup.err
}

// episodeLookup is one episode read as the memo keeps it: the resolution, or the failure that
// left it empty. The failure is kept so a second reader inside the memo's life is told the
// episodes are unknown, not that the series has none.
type episodeLookup struct {
	resolution schedule.EpisodeResolution
	err        error
}

// memoEpisodes records an episode read in the in-process memo. The empty result is memoized
// too — see itemDuration for why remembering a miss matters as much as remembering a hit.
func (s *storeAvailability) memoEpisodes(libraryID string, lookup episodeLookup, now time.Time) {
	s.mu.Lock()
	s.epsMemo[libraryID] = memoEntry[episodeLookup]{val: lookup, exp: now.Add(memoTTL)}
	s.mu.Unlock()
}

// availabilityRun is the availability one reconcile reads. It looks titles up under the
// reconcile's own context and remembers the first read that failed, because the scheduler's
// seam can only say "available" or "not": without this, a failed read reaches the scheduler as
// "not in the library" and the reconcile commits a lineup with that title gone.
type availabilityRun struct {
	from *storeAvailability
	ctx  context.Context

	mu  sync.Mutex
	err error
}

func (r *availabilityRun) Resolve(key provision.Key) (string, int64, bool) {
	id, durationMs, available, err := r.from.resolve(r.ctx, key)
	r.note(err)
	return id, durationMs, available
}

func (r *availabilityRun) ResolveEpisodes(key provision.Key) schedule.EpisodeResolution {
	resolution, err := r.from.resolveEpisodes(r.ctx, key)
	r.note(err)
	return resolution
}

func (r *availabilityRun) note(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	if r.err == nil {
		r.err = err
	}
	r.mu.Unlock()
}

// failure is the first read that failed during the run, or nil when every answer is known.
func (r *availabilityRun) failure() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// reconcileAvailability is the availability a reconcile computes its lineup from, and a check
// to make once it has: an error means a read failed and the lineup must not be committed. Only
// the store-backed availability reads anything; a fixed set of answers cannot fail.
func (e *Engine) reconcileAvailability(ctx context.Context) (schedule.Availability, func() error) {
	fromStore, ok := e.avail.(*storeAvailability)
	if !ok {
		return e.avail, func() error { return nil }
	}
	run := &availabilityRun{from: fromStore, ctx: ctx}
	return run, run.failure
}

// RollingWindow is the rolling-window length a channel with this policy resolves to at `at` (rule
// > channel > the live default), and the wall clock its grid is laid on: exactly what reconcile
// resolves. Pure and cheap, so the playout resolver can apply the window turn (#1675) on every
// airing lookup without arranging a cycle.
func (e *Engine) RollingWindow(policy schedule.ChannelPolicy, at time.Time) (time.Duration, *time.Location) {
	cal := e.Calendar()
	ch := schedule.Channel{DefaultWindow: e.defaultWindowFor()}.WithCalendar(cal)
	return schedule.ResolveWindow(ch, policy, at), cal.Zone
}

// Calendar is the household's holiday calendar: its country's holidays on the window grid's wall
// clock, the one every arrangement this engine computes reads (#2030).
func (e *Engine) Calendar() schedule.Calendar { return e.calendarFor() }

// CarriesOver reports whether the channel's rolling window turns with carry-over (#1675), which
// only internal playout can do: it walks the accepted cycle from an anchor reconcile moves.
// Tunarr loops the list it was given from its own clock and cannot be told where a window
// starts (#1691). The backend is resolved as reconcile resolves it, so both agree on which channels turn.
func (e *Engine) CarriesOver(ctx context.Context, policy schedule.ChannelPolicy) (bool, error) {
	backend, err := e.playoutBackendFor(ctx)
	if err != nil {
		return false, fmt.Errorf("resolve playout backend: %w", err)
	}
	return schedule.PlaysInternally(policy, backend), nil
}

// lastAiredFor loads the channel's airing history for recency-aware placement (§3.1), as of the
// start of the rolling window being arranged: ch.WindowOpened when set, else the window `at`
// falls in. `ch` must already carry DefaultWindow and WindowZone, so the window resolves exactly
// as ComputeDesiredAt will resolve it.
//
// AS OF THE WINDOW START (#1674): the history is the one input to a window's arrangement that
// changes while the window airs, since every tune-in records an airing. Read live, each reconcile
// re-sorted the deck and the same wall-clock instant mapped to a different programme. Read as of
// the boundary, it is constant for the whole window, like the slice offset, and what aired during
// the window shapes the next one.
//
// An UNBOUNDED window never opens a new window, so there is no boundary at which a re-sort could
// land without rewriting what is on air: the deck loops from the channel's anchor forever. Such a
// channel keeps its seeded order and reads no history.
//
// BEST-EFFORT: a store that cannot answer yields an empty map, and placement falls back to
// positional rotation. A history read must never be able to
// fail a reconcile — the channel still has to air something.
//
// Called from BOTH reconcile and CyclePreview, through this one helper, so the preview cannot
// disagree with what actually ships (§8.1's one-code-path rule).
//
// "Aired" means both (#1677, #1678 decision 1): what was watched (the airings playout records) and
// what the schedule put on air (the scheduled ledger reconcile records, plus the accepted cycle's
// own programmes that started before the window opened). Each unit takes its latest of the two, so
// a channel nobody watches still knows what the window before it aired. The accepted cycle is read
// in memory as well as from the ledger, so a window arranged before reconcile has recorded the old
// cycle's last programmes (the playout resolver's turn) is arranged exactly as reconcile will.
func (e *Engine) lastAiredFor(ctx context.Context, ch schedule.Channel, policy schedule.ChannelPolicy, at time.Time, accepted []schedule.Slot, acceptedAnchor time.Time) map[string]time.Time {
	if e.store == nil {
		return nil
	}
	window := schedule.ResolveWindow(ch, policy, at)
	if window <= 0 {
		return nil
	}
	opened := ch.WindowOpened
	if opened.IsZero() {
		opened = schedule.WindowStart(at, window, ch.WindowZone)
	}
	if opened.IsZero() {
		return nil
	}
	hist, err := e.store.LastAiredByChannel(ctx, ch.ID, opened)
	if err != nil {
		e.log.Warn("recency: airing history unavailable; placement falls back to positional rotation",
			"channel", ch.ID, "err", err)
		return nil
	}
	scheduled, err := e.store.LastScheduledByChannel(ctx, ch.ID, opened)
	if err != nil {
		e.log.Warn("recency: scheduled ledger unavailable; using watched airings only", "channel", ch.ID, "err", err)
	}
	for _, b := range scheduledPrograms(accepted, acceptedAnchor, opened) {
		if scheduled == nil {
			scheduled = map[string]time.Time{}
		}
		scheduled[b.LibraryItemID] = latest(scheduled[b.LibraryItemID], b.Start)
	}
	for unit, at := range scheduled {
		if hist == nil {
			hist = map[string]time.Time{}
		}
		hist[unit] = latest(hist[unit], at)
	}
	return hist
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// scheduledPrograms is every programme the accepted cycle, walked from its anchor, started before
// `until` (#1677). An empty cycle or no anchor aired nothing.
func scheduledPrograms(slots []schedule.Slot, anchor, until time.Time) []playout.Broadcast {
	if len(slots) == 0 || anchor.IsZero() || !anchor.Before(until) {
		return nil
	}
	var out []playout.Broadcast
	for _, b := range playout.BroadcastsBetween(slots, anchor, anchor, until) {
		if b.Kind == schedule.SlotProgram && b.LibraryItemID != "" && b.Start.Before(until) {
			out = append(out, b)
		}
	}
	return out
}

// recordScheduled writes the scheduled ledger (#1677): every programme the accepted cycle has
// started by now, up to the instant it hands over at the rolling-window boundary. Each row is
// stamped as known from the programme's start, so a window arranged later reads every programme
// that started before it opened, however late the reconcile that records it runs. Idempotent, and
// best-effort: a ledger write never fails a reconcile.
func (e *Engine) recordScheduled(ctx context.Context, ch channelstore.Channel, now time.Time) {
	if e.store == nil || len(ch.Desired) == 0 || ch.PlayoutAnchor.IsZero() {
		return
	}
	until := now.Add(time.Nanosecond) // a programme starting at now has started
	if window, zone := e.RollingWindow(ch.Policy, now); window > 0 {
		if end := playout.CarryOverEnd(ch.Desired, ch.PlayoutAnchor, schedule.NextWindowStart(ch.PlayoutAnchor, window, zone)); end.Before(until) {
			until = end // past the hand-over the next window airs, not this cycle
		}
	}
	for _, b := range scheduledPrograms(ch.Desired, ch.PlayoutAnchor, until) {
		airing := channeldatastore.ScheduledAiring{Key: b.Key, LibraryItemID: b.LibraryItemID, AiredAt: b.Start}
		if err := e.store.RecordScheduledAirings(ctx, ch.ID, []channeldatastore.ScheduledAiring{airing}, b.Start); err != nil {
			e.log.Warn("recency: scheduled airing not recorded", "channel", ch.ID, "item", b.LibraryItemID, "err", err)
			return
		}
	}
}
