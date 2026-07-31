package app

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/reconcile"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/scheduler"
	"github.com/mantonx/loomarr/internal/setup"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// recurateThresholds adapts the live settings resolver to recurate.Thresholds (§8.2). Reads
// PER CALL (resolved.intv resolves live), so raising recurate.min_score_pct / recurate.max_titles
// takes effect without a restart, like every other setting (config-design §3).
type recurateThresholds struct{ set resolved }

func (t recurateThresholds) MinScorePct(context.Context) int {
	return t.set.intv("recurate.min_score_pct")
}
func (t recurateThresholds) MaxTitles(context.Context) int { return t.set.intv("recurate.max_titles") }

// liveTVAdapter adapts setup.LiveTVConnector to the api.LiveTVService interface
// (Connect returns a struct there, a tuple here). Thin, wiring-only.
type liveTVAdapter struct{ c *setup.LiveTVConnector }

func (a liveTVAdapter) Connect(ctx context.Context) (bool, bool, error) {
	res, err := a.c.Connect(ctx)
	return res.TunerAdded, res.ListingAdded, err
}
func (a liveTVAdapter) Reconnect(ctx context.Context) (int, error) {
	res, err := a.c.Reconnect(ctx)
	return res.TunerRemoved, err
}
func (a liveTVAdapter) Wired(ctx context.Context) (bool, error) { return a.c.Wired(ctx) }

// jobsAdapter adapts *scheduler.Scheduler to api.JobService: it maps scheduler.JobStatus →
// api.JobView (so the api package doesn't import scheduler) and the unknown-job error to the
// api sentinel. Thin, wiring-only.
type jobsAdapter struct{ s *scheduler.Scheduler }

func (a jobsAdapter) List(ctx context.Context) ([]api.JobView, error) {
	st, err := a.s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.JobView, 0, len(st))
	for _, j := range st {
		out = append(out, api.JobView{
			Name: j.Name, Title: j.Title, Description: j.Description,
			Schedule: j.Schedule, ScheduleKey: j.ScheduleKey,
			LastRun: j.LastRun, LastResult: j.LastResult, LastError: j.LastError,
			NextRun: j.NextRun, Running: j.Running, Paused: j.Paused,
			DisabledReason: j.DisabledReason,
		})
	}
	return out, nil
}

func (a jobsAdapter) SetPaused(ctx context.Context, name string, paused bool) error {
	switch err := a.s.SetPaused(ctx, name, paused); err {
	case scheduler.ErrUnknownJob:
		return api.ErrJobNotFound
	case scheduler.ErrJobDisabled:
		return api.ErrJobDisabled
	default:
		return err
	}
}

func (a jobsAdapter) Trigger(ctx context.Context, name string) error {
	switch err := a.s.Trigger(ctx, name); {
	case err == scheduler.ErrUnknownJob:
		return api.ErrJobNotFound
	case err == scheduler.ErrJobDisabled:
		return api.ErrJobDisabled
	case err != nil:
		return err
	}
	return nil
}

// episodeResolver adapts the library client's ListEpisodes to the scheduler's
// EpisodeResolver, mapping library.Episode → schedule.ResolvedProgram so a series
// lineup entry expands into its episodes (§9). Keeps the schedule domain free of
// a library dependency.
func episodeResolver(lib *library.Client) channels.EpisodeResolver {
	return func(ctx context.Context, showItemID string) ([]schedule.ResolvedProgram, error) {
		eps, err := lib.ListEpisodes(ctx, showItemID)
		if err != nil {
			return nil, err
		}
		out := make([]schedule.ResolvedProgram, 0, len(eps))
		for _, e := range eps {
			out = append(out, schedule.ResolvedProgram{
				LibraryItemID: e.LibraryItemID,
				Title:         e.Name,
				DurationMs:    e.DurationMs,
				Season:        e.Season,
				Episode:       e.Episode,
				EpisodeEnd:    e.EpisodeEnd, // §5 multi-part: single-file span end
			})
		}
		return out, nil
	}
}

// bulkDurations adapts the library's BULK metadata call to channels.DurationsResolver.
//
// It reuses ItemMetadataByID rather than adding a duration-only endpoint: that call already asks
// for RunTimeTicks, already batches a comma-separated id list, and already de-duplicates and
// pages (maxIDsPerRequest). A separate bulk-duration call would be a second way to ask the media
// server the same question — the §6 one-adapter rule — for no gain.
//
// The metadata it also returns is simply unused here; the cost is bytes on a call the guide's own
// metadata pass makes anyway, against ONE round trip replacing one-per-movie.
func bulkDurations(lib *library.Client) channels.DurationsResolver {
	return func(ctx context.Context, itemIDs []string) (map[string]int64, error) {
		meta, err := lib.ItemMetadataByID(ctx, itemIDs)
		// Partial results ride WITH the error (ItemMetadataByID's contract), so convert whatever
		// arrived before returning: a half-answered bulk call still saves that many round trips.
		out := make(map[string]int64, len(meta))
		for id, m := range meta {
			if m.RuntimeMs > 0 {
				out[id] = m.RuntimeMs
			}
		}
		return out, err
	}
}

// searchAdapter maps catalog.Catalog to api.SearchService (converts candidates
// to the API's dependency-light shape).
type searchAdapter struct{ cat *catalog.Catalog }

func (a searchAdapter) Search(ctx context.Context, q, scope string, limit int) ([]api.SearchCandidate, error) {
	cands, err := a.cat.Search(ctx, q, catalog.ParseScope(scope), limit)
	if err != nil {
		return nil, err
	}
	out := make([]api.SearchCandidate, 0, len(cands))
	for _, c := range cands {
		out = append(out, api.SearchCandidate{
			MediaType: string(c.MediaType), TMDBID: c.TMDBID, TVDBID: c.TVDBID,
			Name: c.Name, Year: c.Year, InLibrary: c.InLibrary, LibraryItemID: c.LibraryItemID,
			Genres: c.Genres, OfficialRating: c.OfficialRating,
		})
	}
	return out, nil
}

// libraryRatings adapts library.Client to channels.RatingResolver: it resolves an
// approved lineup entry's content rating by the entry's Key, for the reconcile-time
// heal of unrated entries (§389 amendment). Reuses LookupDetail — the same query the
// presence backfill uses — so the rating comes from the media server, not a guess.
type libraryRatings struct{ lib *library.Client }

func (a libraryRatings) Rating(ctx context.Context, key provision.Key) (string, bool, error) {
	mt, provider, id, ok := provision.ParseKey(key)
	if !ok {
		return "", false, nil // an unparseable key can't be looked up; leave it unrated
	}
	kind := library.TMDB
	if provider == "tvdb" {
		kind = library.TVDB
	}
	lmt := library.Movie
	if mt == provision.Series {
		lmt = library.Series
	}
	d, present, err := a.lib.LookupDetail(ctx, kind, strconv.Itoa(id), lmt)
	if err != nil || !present {
		return "", false, err
	}
	return d.OfficialRating, true, nil
}

// libraryBoxSets adapts library.Client to channels.BoxSetResolver: it answers "which
// media-server collections is this title in?" for the reconcile-time stamp backing
// scope.collections (programming-design §2.2).
//
// ⚠ It builds a REVERSE INDEX once and caches it, rather than querying per key. The obvious
// per-key implementation does not exist on the wire — the media server answers "what is in
// collection X", never "what collections is item Y in" — so a per-entry resolve would mean
// re-listing every collection's members for every entry, which is the N+1 the whole stamping
// design exists to avoid. One pass over the collections costs 1 + N(collections) calls total.
//
// Membership is indexed under EVERY key form via reconcile.ScanItemKeys, because a member
// carrying both provider ids must be findable under whichever key the lineup entry was born
// with — the same parity problem the availability scan solves, and the same solution.
//
// The cache is refreshed on a TTL: a collection edited in Emby takes effect within it, which
// is consistent with the stamp only being read at reconcile anyway.
type libraryBoxSets struct {
	lib *library.Client
	ttl time.Duration

	mu      sync.Mutex
	index   map[provision.Key][]string
	fetched time.Time
}

func (a *libraryBoxSets) BoxSets(ctx context.Context, key provision.Key) ([]string, bool, error) {
	idx, err := a.ensureIndex(ctx)
	if err != nil {
		return nil, false, err // unresolved this pass; the entry stays unstamped and airs (§2.2 fail-open)
	}
	// A key absent from the index is a REAL answer — "in no collection" — not a failure. It
	// must return ok=true so the stamp settles, or every non-member re-resolves forever.
	return idx[key], true, nil
}

func (a *libraryBoxSets) ensureIndex(ctx context.Context) (map[provision.Key][]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.index != nil && time.Since(a.fetched) < a.ttl {
		return a.index, nil
	}
	colls, err := a.lib.Collections(ctx)
	if err != nil {
		return nil, err
	}
	idx := make(map[provision.Key][]string)
	for _, c := range colls {
		members, err := a.lib.CollectionMembers(ctx, c.ID)
		if err != nil {
			// One unreadable collection must not discard the whole index — the rest is still
			// true, and a missing membership only ever admits a title (§2.2 fail-open).
			continue
		}
		for _, m := range members {
			for _, k := range reconcile.ScanItemKeys(m) {
				idx[k] = append(idx[k], c.ID)
			}
		}
	}
	a.index, a.fetched = idx, time.Now()
	return idx, nil
}

// libraryCollections adapts library.Client to api.CollectionService: the read-only list
// behind the scope.collections picker (programming-design §2.2).
//
// Deliberately NOT sharing libraryBoxSets' cached index. That index is keyed for MEMBERSHIP
// lookups and is TTL-stale by design (membership is only read at reconcile), whereas an
// operator who just made a collection in Emby and opened the picker expects to see it. The
// list is one cheap call; the index is the expensive one.
type libraryCollections struct{ lib *library.Client }

func (a libraryCollections) Collections(ctx context.Context) ([]api.LibraryCollection, error) {
	colls, err := a.lib.Collections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.LibraryCollection, 0, len(colls))
	for _, c := range colls {
		out = append(out, api.LibraryCollection{ID: c.ID, Name: c.Name, ChildCount: c.ChildCount})
	}
	return out, nil
}

// tmdbFranchises adapts the tmdb.Client to channels.FranchiseResolver: it resolves a MOVIE
// entry's TMDB collection (franchise) id from the entry's Key, for the reconcile-time heal
// (§5 franchise ordering). Only a tmdb-keyed movie can be resolved — a tvdb-keyed movie or a
// series returns ok=false (no lookup), so the entry stays ungrouped (harmless).
type tmdbFranchises struct{ tmdb *tmdb.Client }

func (a tmdbFranchises) Collection(ctx context.Context, key provision.Key) (int, bool, error) {
	mt, provider, id, ok := provision.ParseKey(key)
	if !ok || mt == provision.Series || provider != "tmdb" {
		return 0, false, nil // only a tmdb-keyed movie has a resolvable collection
	}
	cid, err := a.tmdb.CollectionID(ctx, mt, id)
	if err != nil {
		return 0, false, err
	}
	return cid, true, nil // ok=true (resolved); cid 0 means "standalone", a settled answer
}

// libraryPresence adapts library.Client.Lookup to catalog.LibraryPresence, so
// discovery can mark titles the library already owns as in-library. Prefers the
// TMDB id (the discovery id space); falls back to TVDB for a series with no tmdb.
type libraryPresence struct{ lib *library.Client }

func (a libraryPresence) Present(ctx context.Context, mt provision.MediaType, tmdbID, tvdbID int) (catalog.Presence, bool, error) {
	kind, id := library.TMDB, strconv.Itoa(tmdbID)
	if tmdbID == 0 && tvdbID != 0 {
		kind, id = library.TVDB, strconv.Itoa(tvdbID)
	}
	lmt := library.Movie
	if mt == provision.Series {
		lmt = library.Series
	}
	// LookupDetail, not Lookup: discovery needs the rating too, or a kids channel of
	// owned-but-unrated titles resolves to nothing (§9 dead air).
	d, present, err := a.lib.LookupDetail(ctx, kind, id, lmt)
	if err != nil || !present {
		return catalog.Presence{}, false, err
	}
	return catalog.Presence{
		LibraryItemID:  d.ID,
		OfficialRating: d.OfficialRating,
		Genres:         d.Genres,
	}, true, nil
}

// noopValidator is the acquisition validator when TMDB isn't configured: it can't
// re-check existence, so it treats every id as existing. The grounding guarantee
// (the id was surfaced by the catalog tool) still holds; only the belt-and-
// suspenders TMDB exists-check is skipped. Configure TMDB_API_KEY to enable it.
type noopValidator struct{}

func (noopValidator) Exists(context.Context, provision.MediaType, int) (bool, error) {
	return true, nil
}

// --- filler bridging adapters (§10) ---
// These translate between the store's Clip/FillerClip types and the filler
// package's port types, keeping filler free of a store/library import (the domain
// stays pure; main does the wiring).
