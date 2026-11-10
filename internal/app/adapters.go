package app

import (
	"context"
	"errors"

	catalogapi "github.com/loomarr/loomarr/internal/api/catalog"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	libraryapi "github.com/loomarr/loomarr/internal/api/library"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/resolvers"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/playout/livetvpublish"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
)

// liveTVAdapter answers setup status for the durably applied tuner/listing target. The explicit
// resolver matters on Postgres: another replica can advance the checkpoint without changing this
// process's controller Runtime, so request-path status must read DurableView instead.
type liveTVAdapter struct {
	c    *livetvpublish.LiveTVConnector
	urls func(context.Context) (tunarr.LiveTVURLs, error)
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
	c    *livetvpublish.LiveTVConnector
	urls func(context.Context) (tunarr.LiveTVURLs, error)
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

// jobsAdapter adapts *jobs.Scheduler to api.JobService: it maps jobs.JobStatus →
// api.JobView (so the api package doesn't import scheduler) and the unknown-job error to the
// api sentinel. Thin, wiring-only.
type jobsAdapter struct{ s *jobs.Scheduler }

func (a jobsAdapter) List(ctx context.Context) ([]system.JobView, error) {
	st, err := a.s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]system.JobView, 0, len(st))
	for _, j := range st {
		out = append(out, system.JobView{
			Name: j.Name, Group: string(j.Group), Title: j.Title, Description: j.Description,
			Schedule: j.Schedule, ScheduleKey: j.ScheduleKey,
			LastRun: j.LastRun, LastResult: j.LastResult, LastError: j.LastError,
			NextRun: j.NextRun, Running: j.Running, Paused: j.Paused,
			DisabledReason: j.DisabledReason, Overdue: j.Overdue,
		})
	}
	return out, nil
}

func (a jobsAdapter) History(ctx context.Context, name string) (system.JobHistoryView, error) {
	history, err := a.s.History(ctx, name)
	if errors.Is(err, jobs.ErrUnknownJob) {
		return system.JobHistoryView{}, system.ErrJobNotFound
	}
	if err != nil {
		return system.JobHistoryView{}, err
	}
	out := system.JobHistoryView{
		WindowStart: history.WindowStart, RunCount: history.RunCount,
		FailureCount: history.FailureCount, AverageDurationMs: history.AverageDurationMs,
		Truncated: history.Truncated, Recent: make([]system.JobExecutionView, 0, len(history.Recent)),
	}
	for _, run := range history.Recent {
		trigger := "scheduled"
		if run.Manual {
			trigger = "manual"
		}
		out.Recent = append(out.Recent, system.JobExecutionView{
			StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, DurationMs: run.DurationMs,
			Result: run.Result, Error: run.Error, Trigger: trigger,
		})
	}
	return out, nil
}

func (a jobsAdapter) SetPaused(ctx context.Context, name string, paused bool) error {
	switch err := a.s.SetPaused(ctx, name, paused); {
	case errors.Is(err, jobs.ErrUnknownJob):
		return system.ErrJobNotFound
	case errors.Is(err, jobs.ErrJobDisabled):
		return system.ErrJobDisabled
	default:
		return err
	}
}

func (a jobsAdapter) Trigger(ctx context.Context, name string) error {
	switch err := a.s.Trigger(ctx, name); {
	case errors.Is(err, jobs.ErrUnknownJob):
		return system.ErrJobNotFound
	case errors.Is(err, jobs.ErrJobDisabled):
		return system.ErrJobDisabled
	case err != nil:
		return err
	}
	return nil
}

// bulkDurations adapts the Library's BULK item metadata read to channels.DurationsResolver, so a
// layout reads every runtime in one query rather than one per movie. The metadata it also returns
// is simply unused here.
func bulkDurations(lib playoutresolve.ItemMetadata) channels.DurationsResolver {
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

// searchAdapter maps titlesearch.Catalog to api.SearchService (converts candidates
// to the API's dependency-light shape).
type searchAdapter struct{ cat *titlesearch.Catalog }

func (a searchAdapter) Search(ctx context.Context, request catalogapi.SearchRequest) ([]httpcore.SearchCandidate, error) {
	var cands []titlesearch.Candidate
	var err error
	if request.Discovery != nil {
		discovery := request.Discovery
		cands, err = a.cat.Discover(ctx, titlesearch.DiscoveryQuery{
			MediaType: provision.MediaType(discovery.MediaType), Keywords: discovery.Keywords, Genres: discovery.Genres,
			YearFrom: discovery.YearFrom, YearTo: discovery.YearTo,
			OriginalLanguage: discovery.OriginalLanguage, OriginCountry: discovery.OriginCountry,
			RuntimeMin: discovery.RuntimeMin, RuntimeMax: discovery.RuntimeMax,
			VoteAverageMin: discovery.VoteAverageMin, VoteCountMin: discovery.VoteCountMin,
			Network: discovery.Network, Cast: discovery.Cast, Creators: discovery.Creators,
		}, request.Limit)
	} else {
		cands, err = a.cat.Search(ctx, request.Query, titlesearch.ParseScope(request.Scope), request.Limit)
		if request.MediaType != "" {
			filtered := make([]titlesearch.Candidate, 0, len(cands))
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
	out := make([]httpcore.SearchCandidate, 0, len(cands))
	for _, c := range cands {
		out = append(out, searchCandidateFromCatalog(c))
	}
	return out, nil
}

type movieCollectionAdapter struct{ resolver *tmdb.CollectionResolver }

func (a movieCollectionAdapter) ResolveMovieCollections(
	ctx context.Context,
	request libraryapi.MovieCollectionRequest,
) (libraryapi.MovieCollectionResolution, error) {
	keys := make([]provision.Key, 0, len(request.Keys))
	for _, key := range request.Keys {
		keys = append(keys, provision.Key(key))
	}
	resolved, err := a.resolver.Resolve(ctx, keys)
	if err != nil {
		return libraryapi.MovieCollectionResolution{}, err
	}
	out := libraryapi.MovieCollectionResolution{
		Collections: make([]libraryapi.MovieCollection, 0, len(resolved.Collections)),
		Complete:    resolved.Complete,
	}
	for _, collection := range resolved.Collections {
		members := make([]httpcore.SearchCandidate, 0, len(collection.Members))
		for _, member := range collection.Members {
			members = append(members, searchCandidateFromCatalog(member))
		}
		out.Collections = append(out.Collections, libraryapi.MovieCollection{
			TMDBID: collection.TMDBID, Name: collection.Name, Members: members,
		})
	}
	return out, nil
}

func searchCandidateFromCatalog(c titlesearch.Candidate) httpcore.SearchCandidate {
	return httpcore.SearchCandidate{
		MediaType: string(c.MediaType), TMDBID: c.TMDBID, TVDBID: c.TVDBID,
		Name: c.Name, Year: c.Year, InLibrary: c.InLibrary, LibraryItemID: c.LibraryItemID,
		Genres: c.Genres, Overview: c.Overview,
		OriginalLanguage: c.OriginalLanguage, OriginCountries: c.OriginCountries,
		RuntimeMinutes: c.RuntimeMinutes, VoteAverage: c.VoteAverage, VoteCount: c.VoteCount,
		Keywords: c.Keywords, Networks: c.Networks, Cast: c.Cast, Creators: c.Creators,
		OfficialRating: c.OfficialRating,
	}
}

// tunarrNumbers adapts a Programmer to approval.NumberSource: it answers "which channel numbers
// does Tunarr already use?" and nothing else (§9 V54).
//
// ⚠ The narrowing is the point, and it lives HERE rather than in the binder. The binder needs one
// question answered — is this integer free? — and taking a whole Programmer to answer it would
// couple channel NUMBERING to the Tunarr adapter's entire surface. Wiring is the composition
// root's job; this is the seam where a `[]ActualChannel` becomes a set of ints.
type tunarrNumbers struct{ prog tunarr.Programmer }

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

// libraryCollections is api.CollectionService: the read-only list behind the scope.collections
// picker (programming-design §2.2): the household's collections (#2151 step 16).
//
// Deliberately NOT sharing resolvers.BoxSets' cached index. That index is keyed for MEMBERSHIP
// lookups and is TTL-stale by design (membership is only read at reconcile), whereas an
// operator who just made a collection and opened the picker expects to see it.
type libraryCollections struct {
	own resolvers.OwnCollections
}

func (a libraryCollections) Collections(ctx context.Context) ([]libraryapi.LibraryCollection, error) {
	own, err := a.own.ListLibraryCollections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]libraryapi.LibraryCollection, 0, len(own))
	for _, c := range own {
		out = append(out, libraryapi.LibraryCollection{ID: c.ID, Name: c.Name, ChildCount: len(c.Members)})
	}
	return out, nil
}
