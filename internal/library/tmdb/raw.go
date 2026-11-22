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
	return imageAt(imageBase, path)
}

// stillBase is TMDB's image CDN at w300, the width of the thumb ladder's first rung.
const stillBase = "https://image.tmdb.org/t/p/w300"

// StillURL is ImageURL for an episode's still (#4476), at TMDB's 300-wide size rather than
// original: an episode row draws its still at about 300 pixels, a library has tens of thousands
// of them, and an original is a 1920 or wider frame of some hundreds of KB that no surface draws
// at that size. The image service offers no rung wider than the original, so the still is served
// at 300.
func StillURL(path string) string {
	return imageAt(stillBase, path)
}

func imageAt(base, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
