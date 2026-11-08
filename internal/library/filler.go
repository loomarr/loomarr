package library

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// runtimeItem mirrors the /Items slice ItemDurationMs needs. RunTimeTicks is the server-probed
// duration in 100-ns units (Emby/Jellyfin convention).
type runtimeItem struct {
	ID           string `json:"Id"`
	RunTimeTicks int64  `json:"RunTimeTicks"`
}

type runtimeItemsResponse struct {
	Items []runtimeItem `json:"Items"`
}

// ticksPerMs converts Emby/Jellyfin RunTimeTicks (100-ns units) to milliseconds.
const ticksPerMs = 10_000

// ItemDurationMs returns a single library item's runtime in milliseconds,
// resolved from the media server's server-probed RunTimeTicks (§9/§10 — the
// core never probes media itself). Used by the scheduler to give a program slot
// a real duration before pushing the lineup to Tunarr (which rejects ≤ 0).
// Returns (0, nil) if the server reports no runtime.
//
// Uses the LIST endpoint `GET /Items?Ids=<id>&Fields=RunTimeTicks` rather than
// `GET /Items/<id>`: Emby 4.10 rejects the bare single-item path with "could not
// be found" unless it's user-scoped (`/Users/<uid>/Items/<id>`), and Loomarr
// authenticates with a token, not a user context. The Ids-filtered list needs no
// user and is shared across the Emby/Jellyfin flavors. (Caught by the live smoke:
// the bare path 404'd → duration 0 → Tunarr rejected the lineup push.)
func (c *Client) ItemDurationMs(ctx context.Context, itemID string) (int64, error) {
	c, err := c.operation()
	if err != nil {
		return 0, err
	}
	q := url.Values{}
	q.Set("Ids", itemID)
	q.Set("Fields", "RunTimeTicks")
	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return 0, err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

	var resp runtimeItemsResponse
	if err := c.do(req, &resp); err != nil {
		return 0, err
	}
	if len(resp.Items) == 0 {
		return 0, nil // item not found / no runtime — caller falls back (never dead air)
	}
	return resp.Items[0].RunTimeTicks / ticksPerMs, nil
}

// virtualFolder is one library as `GET /Library/VirtualFolders` reports it.
//
// ⚠ **That endpoint returns a BARE ARRAY**, not the `{"Items": […]}` envelope every other
// endpoint in this package uses. Captured from Emby 4.10.0.22 on 2026-08-02 rather than written
// from memory — see fixtures/emby/FINDINGS.md, which is also where the `CollectionType` finding
// below is recorded.
type virtualFolder struct {
	Name string `json:"Name"`
	// Locations are the library's folders as the media server sees them. CollectionType is
	// absent for a mixed library (see LibraryFolders).
	Locations      []string `json:"Locations"`
	CollectionType string   `json:"CollectionType"`
}

// LibraryFolders returns the folders of the library with the given display NAME, as the media
// server sees them, so the cutover can rewrite a filler source that names a media-server library
// into the household library's folder (#2151 decision 36).
//
// ⚠ Returns (nil, nil) for a name the server does not have — "found nothing", not a failure.
//
// ⚠ **Matched on NAME ALONE, never filtered by `CollectionType`.** Three of the seven libraries
// in the 2026-08-02 capture omit that key entirely — it is absent for mixed/unclassified
// libraries, which is exactly what a hand-made commercials library usually is.
func (c *Client) LibraryFolders(ctx context.Context, name string) ([]string, error) {
	c, err := c.operation()
	if err != nil {
		return nil, err
	}
	want := strings.TrimSpace(name)
	if want == "" {
		return nil, nil
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/Library/VirtualFolders", nil)
	if err != nil {
		return nil, err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)

	var folders []virtualFolder
	if err := c.do(req, &folders); err != nil {
		return nil, err
	}
	for _, f := range folders {
		// Case-insensitive: the operator typed a name they read off a screen.
		if strings.EqualFold(strings.TrimSpace(f.Name), want) {
			return f.Locations, nil
		}
	}
	return nil, nil
}
