package app

import (
	"context"
	"strconv"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/setup"
)

// liveTVAdapter adapts setup.LiveTVConnector to the api.LiveTVService interface
// (Connect returns a struct there, a tuple here). Thin, wiring-only.
type liveTVAdapter struct{ c *setup.LiveTVConnector }

func (a liveTVAdapter) Connect(ctx context.Context) (bool, bool, error) {
	res, err := a.c.Connect(ctx)
	return res.TunerAdded, res.ListingAdded, err
}
func (a liveTVAdapter) Wired(ctx context.Context) (bool, error) { return a.c.Wired(ctx) }

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
			})
		}
		return out, nil
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
		})
	}
	return out, nil
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
