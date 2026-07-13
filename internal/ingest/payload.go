// Package ingest is the Sonarr/Radarr webhook handler (design §6, §2). It parses
// the pinned webhook shapes (Phase 0 fixtures) and maps them to provisioning
// domain events (§4). It confirms presence via the library before marking a
// title available (§4 invariant 4).
package ingest

import (
	"encoding/json"
	"fmt"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/provision"
)

// arrPayload is the union of the fields we read from Sonarr and Radarr webhooks
// (pinned fixtures). Sonarr sends series+episodes; Radarr sends movie. Both send
// eventType, instanceName, and a downloadId on Grab/Download. Identity keys:
// Radarr remoteMovie.tmdbId; Sonarr series.tvdbId (§6, confirmed in Phase 0 —
// Sonarr's Grab has no top-level remoteSeries, the tvdbId is under series).
type arrPayload struct {
	EventType    string `json:"eventType"`
	InstanceName string `json:"instanceName"`
	DownloadID   string `json:"downloadId"`

	Movie *struct {
		TMDBID int    `json:"tmdbId"`
		Title  string `json:"title"`
		Year   int    `json:"year"`
	} `json:"movie"`
	RemoteMovie *struct {
		TMDBID int `json:"tmdbId"`
	} `json:"remoteMovie"`

	Series *struct {
		TVDBID int    `json:"tvdbId"`
		Title  string `json:"title"`
		Year   int    `json:"year"`
	} `json:"series"`
	Episodes []struct {
		SeasonNumber  int `json:"seasonNumber"`
		EpisodeNumber int `json:"episodeNumber"`
	} `json:"episodes"`
}

// EventType classifies a webhook (§6). "Download" is the import-complete event
// (the naming quirk pinned in Phase 0), NOT "Import".
const (
	evGrab     = "Grab"
	evDownload = "Download" // import complete — §6 naming quirk
	evTest     = "Test"
)

// parsed is the normalized, app-facing view of a webhook.
type parsed struct {
	eventType  string
	app        string // instanceName ("Sonarr"/"Radarr")
	downloadID string
	title      provision.Title
	key        provision.Key
	// isTest / isTracked classify handling: Test acks + records a timestamp;
	// untracked payloads (no usable identity) ack 200 and are ignored (§6).
	isTest    bool
	isTracked bool
}

// parse decodes and normalizes a raw webhook body. It never errors on an
// unknown/minimal payload — untracked and Test payloads are valid and acked
// (§6); only malformed JSON is an error.
func parse(body []byte) (parsed, error) {
	var p arrPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return parsed{}, fmt.Errorf("ingest: bad webhook json: %w", err)
	}

	out := parsed{eventType: p.EventType, app: p.InstanceName, downloadID: p.DownloadID}

	if p.EventType == evTest {
		// Minimal placeholder shape (§6) — do NOT resolve a title. Ack + record.
		out.isTest = true
		return out, nil
	}

	// Build the identity key from whichever payload identity is present.
	switch {
	case p.Movie != nil || p.RemoteMovie != nil:
		tmdb := 0
		if p.RemoteMovie != nil {
			tmdb = p.RemoteMovie.TMDBID
		}
		if tmdb == 0 && p.Movie != nil {
			tmdb = p.Movie.TMDBID
		}
		if tmdb == 0 {
			return out, nil // untracked: no usable id, ack + ignore
		}
		out.title = provision.Title{MediaType: provision.Movie, TMDBID: tmdb}
		if p.Movie != nil {
			out.title.Name, out.title.Year = p.Movie.Title, p.Movie.Year
		}
	case p.Series != nil:
		if p.Series.TVDBID == 0 {
			return out, nil // untracked
		}
		out.title = provision.Title{MediaType: provision.Series, TVDBID: p.Series.TVDBID, Name: p.Series.Title, Year: p.Series.Year}
		for _, e := range p.Episodes {
			out.title.Seasons = append(out.title.Seasons, e.SeasonNumber)
		}
	default:
		return out, nil // untracked payload shape — ack + ignore
	}

	key, err := out.title.Key()
	if err != nil {
		return out, nil // couldn't derive a key: treat as untracked, ack 200
	}
	out.key = key
	out.isTracked = true
	return out, nil
}

// libraryMediaType maps a domain media type to the library lookup type.
func libraryMediaType(mt provision.MediaType) library.MediaType {
	if mt == provision.Series {
		return library.Series
	}
	return library.Movie
}
