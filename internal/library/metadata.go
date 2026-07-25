package library

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// Programme metadata for the XMLTV guide (§9.1).
//
// The guide needs a description, genres, a year and a content rating per programme — the fields
// that turn a bare title into a listing a household can read. Loomarr's scheduler deliberately
// carries almost none of that on a Slot (title, duration, season, episode) because the schedule
// is about WHAT PLAYS WHEN, and per-item display metadata would bloat every lineup row and every
// policy_json blob for data that is already in the media server.
//
// So the guide fetches it, and the only reason that is affordable is BULK: Emby's `/Items?Ids=`
// takes a comma-separated list, so a whole guide is one request. Measured against the dev Emby:
// 120 episodes returned in 24ms. Per-item calls would have been 120 round trips for the same
// data and would have forced a cache — this needs none.

// ItemMetadata is the display metadata for one library item.
type ItemMetadata struct {
	// Overview is the plot description. XMLTV's `<desc>`, and the field that makes a guide
	// entry worth selecting.
	Overview string
	Genres   []string
	Year     int
	// OfficialRating is the content rating ("TV-PG", "R"). Already carried on a LineupEntry for
	// the SERIES, but an episode-level fetch gets it per episode where the server has it.
	OfficialRating string
}

// maxIDsPerRequest bounds one bulk lookup.
//
// Emby accepts a long Ids list, but the whole thing rides in a URL and both servers and reverse
// proxies cap request-line length — a 200-item list of 7-digit ids is already ~1.6KB. 100 keeps
// each URL comfortably short while still making a full day's guide one or two requests rather
// than hundreds.
const maxIDsPerRequest = 100

// ItemMetadataByID fetches display metadata for many items at once, keyed by item id.
//
// Missing ids are simply absent from the result rather than an error: a guide must render even
// when one item has been removed from the library since the lineup was built, and the caller
// falls back to the title it already has.
//
// A request failure returns what was gathered so far WITH the error, so a caller that prefers a
// thinner guide to no guide can use the partial result. That is the right default here — an EPG
// with titles but no descriptions is far better than an empty one.
func (c *Client) ItemMetadataByID(ctx context.Context, itemIDs []string) (map[string]ItemMetadata, error) {
	out := make(map[string]ItemMetadata, len(itemIDs))
	if len(itemIDs) == 0 {
		return out, nil
	}

	// De-duplicate: a channel airing the same film twice in a cycle, or several channels
	// sharing a title, would otherwise ask for it repeatedly.
	seen := make(map[string]bool, len(itemIDs))
	uniq := make([]string, 0, len(itemIDs))
	for _, id := range itemIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		uniq = append(uniq, id)
	}

	for start := 0; start < len(uniq); start += maxIDsPerRequest {
		end := start + maxIDsPerRequest
		if end > len(uniq) {
			end = len(uniq)
		}

		q := url.Values{}
		q.Set("Ids", strings.Join(uniq[start:end], ","))
		q.Set("Fields", "Overview,Genres,ProductionYear,OfficialRating")
		req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
		if err != nil {
			return out, err
		}
		c.flavor.applyTokenAuth(req, c.token(), c.deviceID)

		var resp metadataItemsResponse
		if err := c.do(req, &resp); err != nil {
			// Partial results are useful — see the doc comment.
			return out, err
		}
		for _, it := range resp.Items {
			out[it.ID] = ItemMetadata{
				Overview:       it.Overview,
				Genres:         it.Genres,
				Year:           it.ProductionYear,
				OfficialRating: it.OfficialRating,
			}
		}
	}
	return out, nil
}

type metadataItemsResponse struct {
	Items []struct {
		ID             string   `json:"Id"`
		Overview       string   `json:"Overview"`
		Genres         []string `json:"Genres"`
		ProductionYear int      `json:"ProductionYear"`
		OfficialRating string   `json:"OfficialRating"`
	} `json:"Items"`
}
