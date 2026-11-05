package tmdb

import (
	"context"
	"strings"
)

// GetJSON performs an authenticated GET of path (including its query string) and decodes the
// 2xx body into out. It shares the client's credential snapshot, transport and observability
// with every other operation, so a caller that reads TMDB's detail endpoints does not need a
// second client. A non-2xx answer is a *StatusError, which FailureClass buckets; a missing key
// is ErrAPIKeyRequired.
func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	ctx, err := c.operation(ctx)
	if err != nil {
		return err
	}
	return c.get(ctx, path, out)
}

// ImageURL returns the absolute original-size URL of an image path TMDB returned (such as
// "/abc.jpg"), or "" when path is empty. These URLs are stored as artwork sources and adopted
// server-side; no page loads them.
func ImageURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return imageBase + path
}
