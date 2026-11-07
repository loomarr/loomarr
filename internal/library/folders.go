package library

import (
	"context"
	"net/http"
)

// notVideoLibraries are the library kinds whose folders hold no shows or movies to scan.
// A mixed library has no CollectionType at all, so it is kept.
var notVideoLibraries = map[string]bool{
	"music": true, "audiobooks": true, "books": true, "photos": true, "boxsets": true, "playlists": true,
}

// MediaFolders lists the folders of the media server's video libraries, as the media server sees
// them, for discovering scan roots (#2151 decision 6). A library's kind decides only whether it
// could hold video; a mixed library is included.
func (c *Client) MediaFolders(ctx context.Context) ([]string, error) {
	c, err := c.operation()
	if err != nil {
		return nil, err
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
	var out []string
	for _, f := range folders {
		if notVideoLibraries[f.CollectionType] {
			continue
		}
		out = append(out, f.Locations...)
	}
	return out, nil
}

// FlavorName is the media server's kind ("emby" or "jellyfin"), naming it as an authority.
func (c *Client) FlavorName() (string, error) {
	c, err := c.operation()
	if err != nil {
		return "", err
	}
	return string(c.flavor()), nil
}
