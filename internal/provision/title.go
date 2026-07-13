// Package provision is the provisioner domain (design §3–§4): the Title/Key
// identity model and the acquisition state machine. It is pure — no I/O, no
// clock, no store. Transitions take an explicit `now` so they stay deterministic
// and exhaustively testable; the store, reconciler, and adapters (Phases 3, 6, 7)
// supply the outside world.
package provision

import (
	"fmt"
	"strconv"
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

// KeyFromWebhook builds the same Key from the identity a Sonarr/Radarr webhook
// carries (design §3 parity requirement; §6 fixtures). Radarr sends
// remoteMovie.tmdbId; Sonarr sends remoteSeries.tvdbId. Passing the ids the
// handler (Phase 6) extracts here guarantees a webhook resolves to the record a
// Title-based request created.
func KeyFromWebhook(mt MediaType, tmdbID, tvdbID int) (Key, error) {
	return Title{MediaType: mt, TMDBID: tmdbID, TVDBID: tvdbID}.Key()
}
