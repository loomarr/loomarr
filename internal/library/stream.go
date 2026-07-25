package library

import (
	"net/url"
	"strings"
)

// Resolving a library item to something ffmpeg can read (§9.1, fact T1).
//
// T1 was: "Loomarr does not know where media lives" — LibraryItem carries an id, a rating
// and genres, no path. That never mattered while Tunarr did the playing, because Tunarr
// resolved paths itself. Internal playout needs a real ffmpeg input.
//
// Two options existed, and the media server's own HTTP endpoint wins decisively:
//
//	(a) The filesystem path. Emby will hand it over via `Fields=Path`, and it is useless to
//	    us: it is EMBY'S view of the filesystem. On the dev setup Emby runs on a different
//	    host entirely, and even co-located it would demand the operator mount media
//	    identically into both containers — the fragile coupling Tunarr deliberately avoids.
//
//	(b) `GET /Videos/{id}/stream?static=true`. ffmpeg reads it over HTTP. No shared mounts,
//	    works across hosts, and it reuses the token the client already holds.
//
// Verified against the live dev Emby: a 4K DV/HDR10 HEVC remux with three audio tracks
// streams over this URL and normalizes to 720p H.264 + AAC stereo.

// StreamURL returns a URL ffmpeg can read for a library item.
//
// `static=true` asks the media server for the ORIGINAL file, not a transcode. That matters:
// playout does its own normalizing to a single profile (§9.1), and letting the media server
// transcode first would mean two encodes in series — twice the CPU for a worse picture.
//
// The token rides in the query string rather than a header because the consumer is ffmpeg,
// which is handed a URL and nothing else. That is the same reason playout's own segment URLs
// carry `playout_token` (§11 device auth): a process or an appliance given a URL cannot set
// headers.
//
// Returns "" when the media server is unconfigured — callers should treat that as "cannot
// play this", not as a relative URL, since an empty base would produce a request to
// ourselves.
func (c *Client) StreamURL(itemID string) string {
	base := c.baseURL()
	if base == "" || itemID == "" {
		return ""
	}
	q := url.Values{}
	q.Set("static", "true")
	if t := c.token(); t != "" {
		// api_key is the query-string form both Emby and Jellyfin accept. The
		// header-based form (X-Emby-Token / MediaBrowser) is unavailable here — see above.
		q.Set("api_key", t)
	}
	return base + "/Videos/" + url.PathEscape(itemID) + "/stream?" + q.Encode()
}

// RedactStreamURL strips the token from a stream URL for logging.
//
// A playout log line naturally wants to say which URL it is reading, and that URL carries a
// credential. The settings Redactor (config-design §4) scrubs known secret VALUES from log
// output, but a URL assembled at call time is easy to hand to a log before it reaches that
// path — so this makes the safe form cheap to reach for.
func RedactStreamURL(raw string) string {
	i := strings.Index(raw, "api_key=")
	if i < 0 {
		return raw
	}
	end := strings.IndexByte(raw[i:], '&')
	if end < 0 {
		return raw[:i] + "api_key=‹redacted›"
	}
	return raw[:i] + "api_key=‹redacted›" + raw[i+end:]
}
