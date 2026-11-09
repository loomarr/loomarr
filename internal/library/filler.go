package library

import (
	"context"
	"net/http"
	"strings"
)

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
