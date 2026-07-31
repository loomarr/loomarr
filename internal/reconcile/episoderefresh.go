package reconcile

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// EpisodeRefresh keeps the cached series episode lists current (§5, §18.1).
//
// WHY IT IS ITS OWN JOB. The obvious home was a hook on `library-scan`, and that would not
// work: LibraryScan correlates only *in-flight* acquisitions (`requested`/`downloading`) and
// returns early when there are none, so a show that is already `available` — precisely the ones
// the guide expands — is never revisited. Attaching episode refresh there would produce a cache
// that looks invalidated and never is, on exactly the settled installs where the guide is used
// most.
//
// WHAT IT REFRESHES. Only the shows referenced by channel lineups, not the whole library. That
// set is small and known, and it is the only set whose staleness anyone can observe: an episode
// list nobody schedules is a row nobody reads.
//
// New episodes still appear promptly by the other path — a newly-landed episode arrives as an
// acquisition, which already triggers a reconcile and a `channel` SSE frame. This job covers
// what nothing else does: episodes added to the media server directly, with no Loomarr
// acquisition behind them.
// EpisodeStore is the slice of the store this job needs. It spans three domains —
// series-episode rows, the channels that reference them, and the titles they resolve to —
// which is why it is declared HERE rather than reached for as one of store's per-domain
// groups: the shape belongs to this job, not to a table.
type EpisodeStore interface {
	ListStaleSeriesEpisodes(ctx context.Context, before time.Time, limit int) ([]store.SeriesEpisodes, error)
	UpsertSeriesEpisodes(ctx context.Context, se store.SeriesEpisodes) error
	ListChannels(ctx context.Context) ([]store.Channel, error)
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
}

type EpisodeRefresh struct {
	store EpisodeStore
	// episodes enumerates a show from the library. Injected (the same resolver the scheduler
	// uses) so this package needs no library client and tests need no live server.
	episodes func(ctx context.Context, showItemID string) ([]schedule.ResolvedProgram, error)
	// maxAge is how stale a cached list may be before it is re-enumerated (`episodes.max_age`).
	maxAge func() time.Duration
	now    func() time.Time
	log    *slog.Logger
}

// NewEpisodeRefresh builds the refresh job. `episodes` may be nil (no library wired), in which
// case Run is a no-op rather than an error: an install without a media server has nothing to
// refresh, and a job that fails every tick would fill the Tasks page with red for no reason.
func NewEpisodeRefresh(
	st EpisodeStore,
	episodes func(ctx context.Context, showItemID string) ([]schedule.ResolvedProgram, error),
	maxAge func() time.Duration,
	now func() time.Time,
	log *slog.Logger,
) *EpisodeRefresh {
	if now == nil {
		now = time.Now
	}
	return &EpisodeRefresh{store: st, episodes: episodes, maxAge: maxAge, now: now, log: log}
}

// refreshLimit caps one run. A bound rather than "everything stale": the job holds no lock, and
// a first run on a large install should not issue hundreds of media-server calls in one burst.
// Stale rows are taken oldest-first, so a capped run still makes progress and the next tick
// picks up where it left off.
const refreshLimit = 50

// Run re-enumerates the stale shows that channel lineups actually reference.
//
// Returns the number of shows refreshed. An error from ONE show is logged and skipped, never
// returned: a single unreachable show must not abort the sweep and leave the rest stale.
func (r *EpisodeRefresh) Run(ctx context.Context) (int, error) {
	if r.episodes == nil {
		return 0, nil // no library wired — nothing to refresh (see the constructor)
	}

	// Which shows are actually scheduled? Anything else is a row nobody reads.
	wanted, err := r.showsInLineups(ctx)
	if err != nil {
		return 0, err
	}
	if len(wanted) == 0 {
		return 0, nil
	}

	now := r.now()
	age := 24 * time.Hour
	if r.maxAge != nil {
		if d := r.maxAge(); d > 0 {
			age = d
		}
	}

	stale, err := r.store.ListStaleSeriesEpisodes(ctx, now.Add(-age), refreshLimit)
	if err != nil {
		return 0, err
	}

	refreshed := 0
	for _, se := range stale {
		if !wanted[se.LibraryID] {
			continue // cached, but no channel schedules it — leave it to age out
		}
		eps, err := r.episodes(ctx, se.LibraryID)
		if err != nil {
			// Logged, not returned: one unreachable show must not abort the sweep. The row
			// keeps its old contents and its old fetched_at, so the next run retries it.
			if r.log != nil {
				r.log.Warn("episode refresh: enumerate failed", "show", se.LibraryID, "err", err)
			}
			continue
		}
		if err := r.store.UpsertSeriesEpisodes(ctx, store.SeriesEpisodes{
			LibraryID: se.LibraryID, Episodes: eps, FetchedAt: now,
		}); err != nil {
			if r.log != nil {
				r.log.Error("episode refresh: persist", "show", se.LibraryID, "err", err)
			}
			continue
		}
		refreshed++
	}
	if refreshed > 0 && r.log != nil {
		r.log.Info("episode refresh", "shows", refreshed, "stale_seen", len(stale))
	}
	return refreshed, nil
}

// showsInLineups is the set of library ids for SERIES entries across every channel's lineup.
//
// Resolved through the title records, because a lineup entry carries a provisioning Key while
// the cache is keyed by the media server's library id — the same duality scan.go documents (one
// show reachable as either `series:tvdb:…` or `series:tmdb:…`).
func (r *EpisodeRefresh) showsInLineups(ctx context.Context) (map[string]bool, error) {
	channels, err := r.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	seen := map[provision.Key]bool{}
	for _, ch := range channels {
		for _, e := range ch.Lineup {
			if !e.Key.IsSeries() || seen[e.Key] {
				continue
			}
			seen[e.Key] = true
			rec, err := r.store.GetTitle(ctx, e.Key)
			if err != nil || rec.State != provision.Available || rec.LibraryID == "" {
				continue
			}
			out[rec.LibraryID] = true
		}
	}
	return out, nil
}
