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
}

// episodeItem mirrors the /Items slice fields we need. RunTimeTicks is the
// server-probed duration (100-ns units, ÷ticksPerMs → ms, same as filler).
type episodeItem struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RunTimeTicks int64  `json:"RunTimeTicks"`
	SeasonNumber *int   `json:"ParentIndexNumber"`
	EpisodeNum   *int   `json:"IndexNumber"`
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
	q := url.Values{}
	q.Set("ParentId", showItemID)
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", "Episode")
	q.Set("Fields", "RunTimeTicks")
	q.Set("SortBy", "ParentIndexNumber,IndexNumber")
	q.Set("SortOrder", "Ascending")

	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)

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
		e := Episode{LibraryItemID: it.ID, Name: it.Name, DurationMs: dur}
		if it.SeasonNumber != nil {
			e.Season = *it.SeasonNumber
		}
		if it.EpisodeNum != nil {
			e.Episode = *it.EpisodeNum
		}
		eps = append(eps, e)
	}
	return eps, nil
}
