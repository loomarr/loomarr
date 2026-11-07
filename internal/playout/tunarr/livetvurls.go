package tunarr

import "strings"

// LiveTVURLs is a Live TV M3U playlist + XMLTV guide pair the media server consumes (§6).
// Tunarr serves one pair (LiveTVURLsAt); Loomarr's internal publisher serves the same shape.
type LiveTVURLs struct {
	M3U   string
	XMLTV string
}

// LiveTVURLsAt derives Tunarr's tuner + guide URLs from a Tunarr base URL.
func LiveTVURLsAt(baseURL string) LiveTVURLs {
	base := strings.TrimRight(baseURL, "/")
	return LiveTVURLs{
		M3U:   base + "/api/channels.m3u",
		XMLTV: base + "/api/xmltv.xml",
	}
}

// LiveTVURLs returns Tunarr's own Live TV URL pair, derived from the live-configured base
// URL (§6). Reads config() directly (no ctx needed): the derivation is a pure string
// transform of BaseURL, not a Tunarr request.
func (t *Tunarr) LiveTVURLs() LiveTVURLs {
	return LiveTVURLsAt(t.config().BaseURL)
}
