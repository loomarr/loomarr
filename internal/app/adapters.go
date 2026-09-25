package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/moviecollections"
	"github.com/loomarr/loomarr/internal/programmer"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/reconcile"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/scheduler"
	"github.com/loomarr/loomarr/internal/setup"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// recurateThresholds adapts the live settings resolver to recurate.Thresholds (§8.2). Reads
// PER CALL (resolved.intv resolves live), so raising recurate.min_score_pct / recurate.max_titles
// takes effect without a restart, like every other setting (config-design §3).
type recurateThresholds struct{ set resolved }

func (t recurateThresholds) MinScorePct(context.Context) int {
	return t.set.intv("recurate.min_score_pct")
}
func (t recurateThresholds) MaxTitles(context.Context) int { return t.set.intv("recurate.max_titles") }

// liveTVAdapter answers setup status for the durably applied tuner/listing target. The explicit
// resolver matters on Postgres: another replica can advance the checkpoint without changing this
// process's controller Runtime, so request-path status must read DurableView instead.
type liveTVAdapter struct {
	c    *setup.LiveTVConnector
	urls func(context.Context) (setup.LiveTVURLs, error)
}

func (a liveTVAdapter) Wired(ctx context.Context) (bool, error) {
	if a.c == nil || a.urls == nil {
		return false, nil
	}
	urls, err := a.urls(ctx)
	if err != nil {
		return false, err
	}
	return a.c.WiredTarget(ctx, urls)
}

// transportTunerRescanner refreshes the tuner catalog selected by the durable
// transport checkpoint. This differs intentionally from LiveTVConnector's ordinary
// applied-configuration resolver during preparation: internal transport opens before
// it becomes the applied UI backend, and pause/detach must disappear from that prepared
// M3U rather than pointlessly rescanning the still-applied Tunarr tuner.
type transportTunerRescanner struct {
	c    *setup.LiveTVConnector
	urls func(context.Context) (setup.LiveTVURLs, error)
}

func (r transportTunerRescanner) RescanTuner(ctx context.Context) error {
	if r.c == nil || r.urls == nil {
		return nil
	}
	urls, err := r.urls(ctx)
	if err != nil {
		return err
	}
	return r.c.RescanTarget(ctx, urls)
}

func (r transportTunerRescanner) PokeGuideRefresh(ctx context.Context) error {
	if r.c == nil {
		return nil
	}
	return r.c.PokeGuideRefresh(ctx)
}

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
			Name: j.Name, Group: string(j.Group), Title: j.Title, Description: j.Description,
			Schedule: j.Schedule, ScheduleKey: j.ScheduleKey,
			LastRun: j.LastRun, LastResult: j.LastResult, LastError: j.LastError,
			NextRun: j.NextRun, Running: j.Running, Paused: j.Paused,
			DisabledReason: j.DisabledReason, Overdue: j.Overdue,
		})
	}
	return out, nil
}

func (a jobsAdapter) History(ctx context.Context, name string) (api.JobHistoryView, error) {
	history, err := a.s.History(ctx, name)
	if err == scheduler.ErrUnknownJob {
		return api.JobHistoryView{}, api.ErrJobNotFound
	}
	if err != nil {
		return api.JobHistoryView{}, err
	}
	out := api.JobHistoryView{
		WindowStart: history.WindowStart, RunCount: history.RunCount,
		FailureCount: history.FailureCount, AverageDurationMs: history.AverageDurationMs,
		Truncated: history.Truncated, Recent: make([]api.JobExecutionView, 0, len(history.Recent)),
	}
	for _, run := range history.Recent {
		trigger := "scheduled"
		if run.Manual {
			trigger = "manual"
		}
		out.Recent = append(out.Recent, api.JobExecutionView{
			StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, DurationMs: run.DurationMs,
			Result: run.Result, Error: run.Error, Trigger: trigger,
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
				Year:          e.ProductionYear,
				Episode:       e.Episode,
				EpisodeEnd:    e.EpisodeEnd, // §5 multi-part: single-file span end
				// Normalized HERE, at the edge, exactly as the binder does for a lineup entry —
				// so the schedule domain only ever compares ladder values (§4).
				OfficialRating:  schedule.NormalizeRating(e.OfficialRating),
				CommunityRating: e.CommunityRating,
				Overview:        e.Overview,
				Tags:            append([]string(nil), e.Tags...),
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

func (a searchAdapter) Search(ctx context.Context, request api.SearchRequest) ([]api.SearchCandidate, error) {
	var cands []catalog.Candidate
	var err error
	if request.Discovery != nil {
		discovery := request.Discovery
		cands, err = a.cat.Discover(ctx, catalog.DiscoveryQuery{
			MediaType: provision.MediaType(discovery.MediaType), Keywords: discovery.Keywords, Genres: discovery.Genres,
			YearFrom: discovery.YearFrom, YearTo: discovery.YearTo,
			OriginalLanguage: discovery.OriginalLanguage, OriginCountry: discovery.OriginCountry,
			RuntimeMin: discovery.RuntimeMin, RuntimeMax: discovery.RuntimeMax,
			VoteAverageMin: discovery.VoteAverageMin, VoteCountMin: discovery.VoteCountMin,
			Network: discovery.Network, Cast: discovery.Cast, Creators: discovery.Creators,
		}, request.Limit)
	} else {
		cands, err = a.cat.Search(ctx, request.Query, catalog.ParseScope(request.Scope), request.Limit)
		if request.MediaType != "" {
			filtered := make([]catalog.Candidate, 0, len(cands))
			for _, candidate := range cands {
				if candidate.MediaType == provision.MediaType(request.MediaType) {
					filtered = append(filtered, candidate)
				}
			}
			cands = filtered
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]api.SearchCandidate, 0, len(cands))
	for _, c := range cands {
		out = append(out, searchCandidateFromCatalog(c))
	}
	return out, nil
}

type movieCollectionAdapter struct{ resolver *moviecollections.Resolver }

func (a movieCollectionAdapter) ResolveMovieCollections(
	ctx context.Context,
	request api.MovieCollectionRequest,
) (api.MovieCollectionResolution, error) {
	keys := make([]provision.Key, 0, len(request.Keys))
	for _, key := range request.Keys {
		keys = append(keys, provision.Key(key))
	}
	resolved, err := a.resolver.Resolve(ctx, keys)
	if err != nil {
		return api.MovieCollectionResolution{}, err
	}
	out := api.MovieCollectionResolution{
		Collections: make([]api.MovieCollection, 0, len(resolved.Collections)),
		Complete:    resolved.Complete,
	}
	for _, collection := range resolved.Collections {
		members := make([]api.SearchCandidate, 0, len(collection.Members))
		for _, member := range collection.Members {
			members = append(members, searchCandidateFromCatalog(member))
		}
		out.Collections = append(out.Collections, api.MovieCollection{
			TMDBID: collection.TMDBID, Name: collection.Name, Members: members,
		})
	}
	return out, nil
}

func searchCandidateFromCatalog(c catalog.Candidate) api.SearchCandidate {
	return api.SearchCandidate{
		MediaType: string(c.MediaType), TMDBID: c.TMDBID, TVDBID: c.TVDBID,
		Name: c.Name, Year: c.Year, InLibrary: c.InLibrary, LibraryItemID: c.LibraryItemID,
		Genres: c.Genres, Overview: c.Overview,
		OriginalLanguage: c.OriginalLanguage, OriginCountries: c.OriginCountries,
		RuntimeMinutes: c.RuntimeMinutes, VoteAverage: c.VoteAverage, VoteCount: c.VoteCount,
		Keywords: c.Keywords, Networks: c.Networks, Cast: c.Cast, Creators: c.Creators,
		OfficialRating: c.OfficialRating,
	}
}

type approvalAdditionAdapter struct {
	presence catalog.LibraryPresenceSource
}

func (a approvalAdditionAdapter) ResolveApprovalAddition(
	ctx context.Context,
	item suggest.ProposalItem,
) (suggest.ProposalItem, bool, error) {
	key, err := (provision.Title{
		MediaType: item.MediaType, TMDBID: item.TMDBID, TVDBID: item.TVDBID,
	}).Key()
	if err != nil {
		return suggest.ProposalItem{}, false, fmt.Errorf("invalid title identity: %w", err)
	}
	mediaType, _, _, ok := provision.ParseKey(key)
	if !ok {
		return suggest.ProposalItem{}, false, errors.New("invalid title key")
	}
	item.InLibrary = false
	item.LibraryItemID = ""
	item.OfficialRating = ""
	if a.presence == nil {
		return item, false, nil
	}
	presence := a.presence()
	if presence == nil {
		return item, false, nil
	}
	owned, found, err := presence.Present(ctx, mediaType, item.TMDBID, item.TVDBID)
	if err != nil {
		return suggest.ProposalItem{}, false, err
	}
	if !found {
		return item, false, nil
	}
	item.InLibrary = true
	item.LibraryItemID = owned.LibraryItemID
	item.OfficialRating = owned.OfficialRating
	item.Genres = append([]string(nil), owned.Genres...)
	return item, true, nil
}

// tunarrNumbers adapts a Programmer to binder.NumberSource: it answers "which channel numbers
// does Tunarr already use?" and nothing else (§9 V54).
//
// ⚠ The narrowing is the point, and it lives HERE rather than in the binder. The binder needs one
// question answered — is this integer free? — and taking a whole Programmer to answer it would
// couple channel NUMBERING to the Tunarr adapter's entire surface. Wiring is the composition
// root's job; this is the seam where a `[]ActualChannel` becomes a set of ints.
type tunarrNumbers struct{ prog programmer.Programmer }

func (a tunarrNumbers) TakenChannelNumbers(ctx context.Context) (map[int]bool, error) {
	chans, err := a.prog.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	used := make(map[int]bool, len(chans))
	for _, c := range chans {
		used[c.Number] = true
	}
	return used, nil
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

	mu         sync.Mutex
	index      map[provision.Key][]string
	fetched    time.Time
	generation library.ConnectionGeneration
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
	lib := a.lib.Snapshot()
	generation, err := lib.Connection().Generation()
	if err != nil {
		return nil, err
	}
	if index, ok := a.cachedIndex(generation, time.Now()); ok {
		return index, nil
	}
	colls, err := lib.Collections(ctx)
	if err != nil {
		return nil, err
	}
	idx := make(map[provision.Key][]string)
	for _, c := range colls {
		members, err := lib.CollectionMembers(ctx, c.ID)
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
	a.index, a.fetched, a.generation = idx, time.Now(), generation
	return idx, nil
}

func (a *libraryBoxSets) cachedIndex(
	generation library.ConnectionGeneration, now time.Time,
) (map[provision.Key][]string, bool) {
	if a.index == nil || a.generation != generation || now.Sub(a.fetched) >= a.ttl {
		return nil, false
	}
	return a.index, true
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
	if errors.Is(err, tmdb.ErrAPIKeyRequired) {
		// The adapter is deliberately wired before TMDB is configured. An absent or
		// freshly cleared key means franchise metadata is unavailable, not that the
		// channel reconcile failed. Returning ok=false leaves the entry unresolved so
		// a later configured reconcile can heal it.
		return 0, false, nil
	}
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

// --- filler bridging adapters (§10) ---
// These translate between the store's Clip/FillerClip types and the filler
// package's port types, keeping filler free of a store/library import (the domain
// stays pure; main does the wiring).
