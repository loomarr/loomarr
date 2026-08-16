package library

import (
	"context"
	"net/http"
	"net/url"
)

// Episode enumeration (§9 series expansion). A series lineup pick is not directly
// playable — its episodes are the programs. ListEpisodes returns a show's episodes
// (id + duration + season/episode) so the scheduler can expand a series entry into
// one program slot per episode. Reuses the same /Items surface as filler/search
// (§6), scoped to the show via ParentId.

// Episode is one playable episode of a series, ordered by season then episode.
type Episode struct {
	LibraryItemID string
	Name          string
	DurationMs    int64
	Season        int
	Episode       int
	// EpisodeEnd is the last episode number this item spans when it's a single file
	// holding a multi-part story (the media server's IndexNumberEnd, e.g. a "25-26"
	// double-episode). 0 when unset (a normal single episode). Used with the title-
	// suffix heuristic to keep two-parters together (§5 multi-part adjacency floor).
	EpisodeEnd int
	// OfficialRating is THIS EPISODE's content rating, which is not always the show's
	// (§4 audience ceiling). "" when the media server has none for the episode.
	//
	// ⚠ The series-level rating is a lossy SUMMARY, and enforcing against it alone let
	// above-ceiling episodes air. Measured on the maintainer's library: King of the Hill
	// is a TV-PG series whose 275 episodes are 253 × TV-PG, 20 × unrated — and 2 × TV-14.
	// Those two aired on a TV-PG channel because nothing below the series entry was ever
	// asked. TMDB agrees the summary is lossy: it lists BOTH TV-PG and TV-14 for the show.
	OfficialRating string
}

// episodeItem mirrors the /Items slice fields we need. RunTimeTicks is the
// server-probed duration (100-ns units, ÷ticksPerMs → ms, same as filler).
type episodeItem struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RunTimeTicks int64  `json:"RunTimeTicks"`
	SeasonNumber *int   `json:"ParentIndexNumber"`
	EpisodeNum   *int   `json:"IndexNumber"`
	EpisodeEnd   *int   `json:"IndexNumberEnd"` // set on a single-file multi-part episode
	// Absent on most episodes even when the SHOW is rated — hence a plain string with ""
	// meaning "the server has none", not "unrated content".
	OfficialRating string `json:"OfficialRating"`
}

type episodesResponse struct {
	Items []episodeItem `json:"Items"`
}

// ListEpisodes enumerates a series' episodes (§9):
//
//	GET /Items?ParentId=<showItemID>&Recursive=true&IncludeItemTypes=Episode
//	           &Fields=RunTimeTicks&SortBy=ParentIndexNumber,IndexNumber
//
// Returned in season/episode order. Duration comes from RunTimeTicks (the core
// never probes media). Episodes with no positive runtime are dropped — a program
// slot needs duration > 0 (Tunarr rejects ≤ 0), so a runtime-less episode can't
// be scheduled.
func (c *Client) ListEpisodes(ctx context.Context, showItemID string) ([]Episode, error) {
	c, err := c.operation()
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("ParentId", showItemID)
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", "Episode")
	q.Set("Fields", "RunTimeTicks,IndexNumberEnd,OfficialRating")
	q.Set("SortBy", "ParentIndexNumber,IndexNumber")
	q.Set("SortOrder", "Ascending")

	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

	var out episodesResponse
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	eps := make([]Episode, 0, len(out.Items))
	for _, it := range out.Items {
		dur := it.RunTimeTicks / ticksPerMs
		if dur <= 0 {
			continue // unplayable as a program slot (Tunarr requires duration > 0)
		}
		e := Episode{LibraryItemID: it.ID, Name: it.Name, DurationMs: dur, OfficialRating: it.OfficialRating}
		if it.SeasonNumber != nil {
			e.Season = *it.SeasonNumber
		}
		if it.EpisodeNum != nil {
			e.Episode = *it.EpisodeNum
		}
		if it.EpisodeEnd != nil {
			e.EpisodeEnd = *it.EpisodeEnd
		}
		eps = append(eps, e)
	}
	return eps, nil
}
