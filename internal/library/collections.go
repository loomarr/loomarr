package library

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Collection is one media-server collection — a BoxSet in Emby/Jellyfin vocabulary
// (design §6 Collections, programming-design §2.2). These are hand-curated shelves
// (an operator's "Halloween" list, or whatever Kometa wrote), which is what makes them
// worth scoping a channel to: membership is an explicit human judgment rather than
// derived metadata.
//
// ⚠ NOT a TMDB collection. `schedule.LineupEntry.CollectionID` is the TMDB *franchise*
// id (§5 ordering) — an integer naming the Alien films. This ID is an opaque
// server-local string naming a shelf someone made. The two namespaces are never
// interchangeable, and comparing one to the other type-checks while matching nothing.
type Collection struct {
	ID         string // media-server item id — opaque, server-local
	Name       string // display label for the picker
	ChildCount int    // members as the server counts them; 0 when the field is absent
	// TMDBID is the TMDB collection a tool generated this one from (one per franchise); "" for a
	// collection someone made by hand.
	TMDBID string
}

// collectionItem mirrors the slice of an /Items BoxSet entry we read.
type collectionItem struct {
	ID          string            `json:"Id"`
	Name        string            `json:"Name"`
	ChildCount  int               `json:"ChildCount"`
	ProviderIDs map[string]string `json:"ProviderIds"`
}

type collectionsResponse struct {
	Items []collectionItem `json:"Items"`
}

// Collections lists the media server's collections (design §6):
//
//	GET /Items?IncludeItemTypes=BoxSet&Recursive=true&Fields=ChildCount&SortBy=SortName
//
// Sorted server-side by name so the picker's order is stable without a client sort.
// Same /Items surface and same flavored header auth as every other call here.
func (c *Client) Collections(ctx context.Context) ([]Collection, error) {
	c, err := c.operation()
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("IncludeItemTypes", "BoxSet")
	q.Set("Recursive", "true")
	q.Set("Fields", "ChildCount,ProviderIds")
	q.Set("SortBy", "SortName")

	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

	var out collectionsResponse
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	results := make([]Collection, 0, len(out.Items))
	for _, it := range out.Items {
		results = append(results, Collection{
			ID: it.ID, Name: it.Name, ChildCount: it.ChildCount, TMDBID: it.ProviderIDs["Tmdb"],
		})
	}
	return results, nil
}

// CollectionMembers returns the titles in one collection (design §6):
//
//	GET /Items?ParentId=<id>&Recursive=true&Fields=ProviderIds
//
// Members come back as ordinary Movie/Series items, so this reuses SearchResult and its
// provider ids map straight onto a provision.Key — the same key-parity property the bulk
// scan relies on, with no second lookup per member.
//
// ⚠ ParentId is the documented membership query but is the one part of this adapter not
// pinned by a live capture (design §6): a BoxSet is not a folder in the library tree the
// way a season sits under a series. If it returns empty against a collection with a
// non-zero ChildCount, THIS query is wrong — not the caller. The failure is loud (an
// empty member list), never a silent mis-filter.
func (c *Client) CollectionMembers(ctx context.Context, collectionID string) ([]SearchResult, error) {
	c, err := c.operation()
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("ParentId", collectionID)
	q.Set("Recursive", "true")
	q.Set("Fields", "ProviderIds,ProductionYear,Genres,Overview,OfficialRating")

	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

	var out collectionMembersResponse
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(out.Items))
	for _, it := range out.Items {
		results = append(results, SearchResult{
			LibraryItemID:  it.ID,
			Name:           it.Name,
			Year:           it.ProductionYear,
			MediaType:      mediaTypeFromEmby(it.Type),
			TMDBID:         atoiSafe(it.ProviderIds.Tmdb),
			TVDBID:         atoiSafe(it.ProviderIds.Tvdb),
			IMDBID:         it.ProviderIds.Imdb,
			Genres:         it.Genres,
			Overview:       it.Overview,
			OfficialRating: it.OfficialRating,
		})
	}
	return results, nil
}

// collectionMember is the slice of an /Items entry the collection import reads.
type collectionMember struct {
	ID             string   `json:"Id"`
	Name           string   `json:"Name"`
	ProductionYear int      `json:"ProductionYear"`
	Type           string   `json:"Type"`
	Genres         []string `json:"Genres"`
	Overview       string   `json:"Overview"`
	OfficialRating string   `json:"OfficialRating"`
	ProviderIds    struct {
		Tmdb string `json:"Tmdb"`
		Tvdb string `json:"Tvdb"`
		Imdb string `json:"Imdb"`
	} `json:"ProviderIds"`
}

type collectionMembersResponse struct {
	Items []collectionMember `json:"Items"`
}

// mediaTypeFromEmby maps Emby's "Type" to our MediaType (Movie/Series).
func mediaTypeFromEmby(t string) MediaType {
	switch t {
	case "Series":
		return Series
	default:
		return Movie
	}
}

// atoiSafe parses an int, returning 0 on any error (a missing/blank provider id).
func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
