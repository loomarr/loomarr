package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/images"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// timelineThumbResolver is the slow half of programme artwork: it turns a programme identity into
// one of OUR image URLs by asking TMDB for the source, adopting it into the image service and
// warming the bytes. It is never on a request path — timelineThumbs runs it in the background and
// serves the answer from memory.
//
// A series episode gets its OWN still (per-episode), a movie its landscape backdrop. The image
// always comes from TMDB, but the KEY may be TVDB (series are usually TVDB-keyed, §3) — so a tvdb
// series bridges TVDB→TMDB via /find first.
//
// ⚠ Since V52 phase 7 the URL it returns is OURS, not TMDB's: the still is adopted into the image
// service and served from our own disk, so the Watch timeline stops loading third-party images in
// the operator's browser (§22).
type timelineThumbResolver struct {
	tmdb   *tmdb.Client
	images *images.Service
	fetch  timelineImageFetcher
}

// Kept as the narrow capability the resolver needs so the cold-image behavior is testable without
// a network. *images.Fetcher is the production implementation shared with the scheduled job.
type timelineImageFetcher interface {
	FetchNow(ctx context.Context, work []images.Image, budget time.Duration) map[string]images.Image
}

// timelineThumbWidth is the rung the strip's `src` points at. The blocks are small — a strip of
// preview chips, not hero art — so this is the bottom of the 16:9 ladder; `thumbImage` carries the
// full srcset for a client that wants a denser rendition.
const timelineThumbWidth = 300

// timelineThumbWarmBudget is the ceiling for downloading one source image in the BACKGROUND. No
// request waits on it, so it can be generous; it only stops a stalled origin holding a warm slot.
const timelineThumbWarmBudget = 15 * time.Second

// thumbFailure tags a non-TMDB failure with a class, so every reason a programme has no image is
// recorded under a name rather than swallowed.
type thumbFailure struct {
	class string
	err   error
}

func (f *thumbFailure) Error() string { return f.class + ": " + f.err.Error() }
func (f *thumbFailure) Unwrap() error { return f.err }

var (
	errThumbNoFetcher   = errors.New("no image fetcher wired")
	errThumbFetchFailed = errors.New("image not fetched within budget")
)

// failureClassOf names why a resolve failed: a TMDB class, or the image pipeline stage.
func failureClassOf(err error) string {
	var f *thumbFailure
	if errors.As(err, &f) {
		return f.class
	}
	return tmdb.FailureClass(err)
}

// resolve returns the image URL and content hash for a programme, or "", "" with a nil error when
// there is legitimately no image (no usable key, TMDB has none, no image service). A non-nil error
// is a FAILURE the caller must record and not retry immediately.
func (a timelineThumbResolver) resolve(ctx context.Context, key string, season, episode int) (string, string, error) {
	mt, provider, id, ok := provision.ParseKey(provision.Key(key))
	if !ok {
		return "", "", nil // no usable key
	}
	var (
		src  string
		err  error
		role = images.RoleBackdrop // an episode still is 16:9
	)
	switch {
	case mt == provision.Series && provider == "tvdb":
		// The common series case (§3: series prefer a TVDB key). Bridge TVDB→TMDB, then the episode
		// still — the strip is per-programme, so the episode's own still, not the show poster.
		src, err = a.tmdb.EpisodeStillURLByTVDB(ctx, id, season, episode)
	case mt == provision.Series: // provider == "tmdb"
		src, err = a.tmdb.EpisodeStillURL(ctx, id, season, episode)
	case provider == "tmdb": // a movie
		// A backdrop is the movie equivalent of an episode still: landscape art for the same
		// 16:9 preview frame. A portrait poster made movie cards narrow or forced a crop.
		src, err = a.tmdb.BackdropURL(ctx, mt, id)
	default:
		return "", "", nil // a tvdb-keyed movie is not a shape we produce
	}
	if err != nil {
		return "", "", err
	}
	if src == "" {
		return "", "", nil
	}
	if a.images == nil {
		// No image service ⇒ no image, NOT a fallback to TMDB's CDN. §22 exists to take third-party
		// origins out of the operator's browser; the strip already renders a fallback for a missing
		// image, which is the honest degradation.
		return "", "", nil
	}

	// Member-visible: the Watch timeline is behind a session, unlike a channel icon that Tunarr
	// fetches unauthenticated (§22 — visibility is a property of the image, not the route).
	rec, err := a.images.Adopt(ctx, src, images.IngestRequest{Role: role, Visibility: images.VisibilityMember})
	if err != nil {
		return "", "", &thumbFailure{class: "image_adopt", err: err}
	}
	// ⚠ **Adopted but not yet fetched ⇒ warm it now, then use the RETURNED row.** FetchNow re-keys
	// the URL-placeholder hash to the content hash, so continuing to use `rec` would mint a URL
	// that 404s as soon as the fetch finishes.
	if rec.OriginFetchedAt.IsZero() {
		if a.fetch == nil {
			return "", "", &thumbFailure{class: "image_fetch", err: errThumbNoFetcher}
		}
		warm := a.fetch.FetchNow(ctx, []images.Image{rec}, timelineThumbWarmBudget)
		rec, ok = warm[src]
		if !ok {
			return "", "", &thumbFailure{class: "image_fetch", err: errThumbFetchFailed}
		}
	}
	// This URL is consumed only by Loomarr's in-app Watch timeline. Keep it on the page's own
	// origin; server.public_url is the machine-client address and may be unreachable from the
	// viewer even while the app itself is open.
	return a.images.PathFor(rec.Hash, timelineThumbWidth, images.FormatJPEG), rec.Hash, nil
}

// Cache lifetimes. A found image is stable for hours; "TMDB has nothing" changes rarely, so it
// keeps a long-ish answer; a FAILURE is retried soon, but not on every Guide load.
const (
	timelineThumbReadyTTL   = 3 * time.Hour
	timelineThumbAbsentTTL  = time.Hour
	timelineThumbFailureTTL = 2 * time.Minute

	// timelineThumbMaxEntries bounds the memo: a household's programme set is a few thousand at
	// most, and the value is two short strings.
	timelineThumbMaxEntries = 4096
	// timelineThumbMaxPending bounds queued background warms. A Guide window asks for ~50 keys, so
	// this is several windows deep; past it a key is simply asked for again on the next request.
	timelineThumbMaxPending = 256
	// timelineThumbWarmers bounds concurrent TMDB/image work, kinder to the provider than the old
	// per-request fan-out of eight.
	timelineThumbWarmers = 4
)

type timelineThumbID struct {
	key             string
	season, episode int
}

type thumbEntry struct {
	url, hash string
	expires   time.Time
}

// timelineThumbs implements api.TimelineThumbResolver for the Guide and Watch timeline.
//
// ThumbFor is a memory read and NOTHING else: no TMDB call, no image fetch, no database access on
// the request path (#1397 — it used to make ~57 outbound calls per Guide load and download cold
// images inline under a 3s budget). A miss returns "" and starts one deduplicated background warm
// on the application context; the artwork shows on a later refresh, which the maintainer accepted
// for a hover enhancement. Answers are cached — found, absent, and failed (short) — so a failing
// key is not retried per request.
type timelineThumbs struct {
	src timelineThumbResolver
	ctx context.Context
	log *slog.Logger
	now func() time.Time

	mu       sync.Mutex
	entries  map[timelineThumbID]thumbEntry
	inflight map[timelineThumbID]struct{}
	slots    chan struct{}
	wg       sync.WaitGroup
}

func newTimelineThumbs(ctx context.Context, src timelineThumbResolver, log *slog.Logger) *timelineThumbs {
	if log == nil {
		log = slog.Default()
	}
	return &timelineThumbs{
		src: src, ctx: ctx, log: log, now: time.Now,
		entries:  make(map[timelineThumbID]thumbEntry),
		inflight: make(map[timelineThumbID]struct{}),
		slots:    make(chan struct{}, timelineThumbWarmers),
	}
}

// ThumbFor serves the cached answer. A stale answer is still served while it refreshes, so a
// thumbnail never blinks out at its TTL.
func (t *timelineThumbs) ThumbFor(_ context.Context, key string, season, episode int) (string, string) {
	if key == "" {
		return "", ""
	}
	id := timelineThumbID{key: key, season: season, episode: episode}
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[id]
	if ok && t.now().Before(e.expires) {
		return e.url, e.hash
	}
	t.scheduleLocked(id)
	return e.url, e.hash // stale-while-revalidate; zero when never resolved
}

// scheduleLocked starts a background warm unless one is already running or the queue is full.
func (t *timelineThumbs) scheduleLocked(id timelineThumbID) {
	if _, running := t.inflight[id]; running || len(t.inflight) >= timelineThumbMaxPending || t.ctx.Err() != nil {
		return
	}
	t.inflight[id] = struct{}{}
	t.wg.Add(1)
	go t.warm(id)
}

func (t *timelineThumbs) warm(id timelineThumbID) {
	defer t.wg.Done()
	select {
	case t.slots <- struct{}{}:
		defer func() { <-t.slots }()
	case <-t.ctx.Done():
		t.finish(id, nil, nil)
		return
	}
	url, hash, err := t.src.resolve(t.ctx, id.key, id.season, id.episode)
	t.finish(id, &thumbEntry{url: url, hash: hash}, err)
}

// finish records the outcome. A failure is logged with its class and cached briefly; if an earlier
// good answer exists it is kept, so a TMDB blip does not remove artwork that already worked.
func (t *timelineThumbs) finish(id timelineThumbID, res *thumbEntry, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.inflight, id)
	if res == nil || t.ctx.Err() != nil {
		return // shutting down — nothing worth caching
	}
	now := t.now()
	switch {
	case err != nil:
		prev := t.entries[id]
		res.url, res.hash = prev.url, prev.hash
		res.expires = now.Add(timelineThumbFailureTTL)
		t.log.Warn("programme artwork lookup failed",
			"class", failureClassOf(err), "key", id.key, "season", id.season, "episode", id.episode, "err", err)
	case res.url == "":
		res.expires = now.Add(timelineThumbAbsentTTL)
	default:
		res.expires = now.Add(timelineThumbReadyTTL)
	}
	t.storeLocked(id, *res)
}

func (t *timelineThumbs) store(id timelineThumbID, e thumbEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.storeLocked(id, e)
}

// storeLocked inserts within the bound: expired entries go first, then an arbitrary one.
func (t *timelineThumbs) storeLocked(id timelineThumbID, e thumbEntry) {
	if _, exists := t.entries[id]; !exists && len(t.entries) >= timelineThumbMaxEntries {
		now := t.now()
		for k, v := range t.entries {
			if !now.Before(v.expires) {
				delete(t.entries, k)
			}
		}
		for k := range t.entries {
			if len(t.entries) < timelineThumbMaxEntries {
				break
			}
			delete(t.entries, k)
		}
	}
	t.entries[id] = e
}

func (t *timelineThumbs) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

// wait blocks until every background warm has finished. Tests only.
func (t *timelineThumbs) wait() { t.wg.Wait() }
