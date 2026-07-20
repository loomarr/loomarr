// Package provision is the provisioner domain (design §3–§4): the Title/Key
// identity model and the acquisition state machine. It is pure — no I/O, no
// clock, no store. Transitions take an explicit `now` so they stay deterministic
// and exhaustively testable; the store, reconciler, and adapters (Phases 3, 6, 7)
// supply the outside world.
package provision

import (
	"fmt"
	"strconv"
	"strings"
)

// MediaType is the kind of content a Title refers to (§3).
type MediaType string

const (
	Movie  MediaType = "movie"
	Series MediaType = "series"
)

// Valid reports whether m is a known media type.
func (m MediaType) Valid() bool { return m == Movie || m == Series }

// Title is a unit of content the app wants (§3). Identity is an external id,
// never the name — Name/Year exist only for logs and request payloads.
type Title struct {
	MediaType MediaType
	TMDBID    int // canonical for movies; accepted by Seerr for series
	TVDBID    int // preferred key for series; 0 = unset
	Name      string
	Year      int
	Seasons   []int // series only; empty = all
}

// Key is the stable dedup/identity key. It MUST be identical whether derived
// from a Title or from an ingest webhook (§3) — that parity is what lets an
// incoming Grab/Import event find the Record its request created.
//
//	series with a TVDB id → "series:tvdb:<id>"
//	otherwise             → "<mediatype>:tmdb:<id>"
type Key string

// IsSeries reports whether the key identifies a series (§3 keys are media-type
// prefixed: "series:tvdb:<id>" / "series:tmdb:<id>" vs "movie:tmdb:<id>"). The
// scheduler uses this to expand a series into its episodes (§9).
func (k Key) IsSeries() bool { return strings.HasPrefix(string(k), string(Series)+":") }

// Key derives the identity key for a Title (§3). It errors rather than produce
// an ambiguous key from an under-identified Title, since a wrong key silently
// breaks webhook correlation.
func (t Title) Key() (Key, error) {
	if !t.MediaType.Valid() {
		return "", fmt.Errorf("invalid media type %q", t.MediaType)
	}
	if t.MediaType == Series && t.TVDBID > 0 {
		return Key("series:tvdb:" + strconv.Itoa(t.TVDBID)), nil
	}
	if t.TMDBID > 0 {
		return Key(string(t.MediaType) + ":tmdb:" + strconv.Itoa(t.TMDBID)), nil
	}
	return "", fmt.Errorf("title %q (%s) has no usable id (need TMDBID, or TVDBID for series)", t.Name, t.MediaType)
}

// ParseKey is the inverse of Title.Key: it recovers (mediaType, provider, id) from
// a Key string ("movie:tmdb:603", "series:tvdb:71663", "series:tmdb:4229"). ok is
// false for a malformed key. Used where a downstream call needs the raw ids back —
// e.g. a library rating lookup for an approved lineup entry, which stores only the
// Key. Provider is "tmdb" or "tvdb".
func ParseKey(k Key) (mt MediaType, provider string, id int, ok bool) {
	parts := strings.Split(string(k), ":")
	if len(parts) != 3 {
		return "", "", 0, false
	}
	mt = MediaType(parts[0])
	if !mt.Valid() {
		return "", "", 0, false
	}
	if parts[1] != "tmdb" && parts[1] != "tvdb" {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil || n <= 0 {
		return "", "", 0, false
	}
	return mt, parts[1], n, true
}

// KeyFromWebhook builds the same Key from the identity a Sonarr/Radarr webhook
// carries (design §3 parity requirement; §6 fixtures). Radarr sends
// remoteMovie.tmdbId; Sonarr sends remoteSeries.tvdbId. Passing the ids the
// handler (Phase 6) extracts here guarantees a webhook resolves to the record a
// Title-based request created.
func KeyFromWebhook(mt MediaType, tmdbID, tvdbID int) (Key, error) {
	return Title{MediaType: mt, TMDBID: tmdbID, TVDBID: tvdbID}.Key()
}
