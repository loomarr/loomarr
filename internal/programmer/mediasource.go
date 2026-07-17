package programmer

import (
	"context"
	"net/http"
	"strings"
)

// This file wires the media server as *Tunarr's* media source (§6): so Tunarr can
// stream the underlying files and index them into its program table. Loomarr does
// this for the operator during setup (POST /v1/setup/tunarr-connect) using its
// existing admin token — Tunarr accepts the admin X-Emby-Token as the source access
// token, so there is no separate Emby user login and Loomarr stores no extra
// credential. Enumerate-first + idempotent, like the Live TV wiring and filler.

type embyMediaSource struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "emby" | "jellyfin" | "plex" | "local"
	URI  string `json:"uri"`
}

type srcLibrary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"` // "movies" | "shows" | "music" | …
	Enabled   bool   `json:"enabled"`
}

// isMediaServer reports a source type Loomarr programs against (movies/shows).
func isMediaServer(sourceType string) bool { return sourceType == "emby" || sourceType == "jellyfin" }

// isProgrammable reports a library Loomarr schedules from (movies + TV episodes).
func isProgrammable(mediaType string) bool { return mediaType == "movies" || mediaType == "shows" }

// EnsureEmbySource ensures a media source of the given flavor ("emby"|"jellyfin")
// pointing at embyURL exists in Tunarr, returning its id. Idempotent — it reuses an
// existing source for embyURL rather than creating a duplicate. token/userID are the
// admin API key + the admin user's id (Tunarr uses them as the source access token).
func (t *Tunarr) EnsureEmbySource(ctx context.Context, flavor, embyURL, token, userID string) (string, error) {
	var existing []embyMediaSource
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources", nil, &existing); err != nil {
		return "", err
	}
	want := strings.TrimRight(embyURL, "/")
	for _, s := range existing {
		if s.Type == flavor && strings.TrimRight(s.URI, "/") == want {
			return s.ID, nil // already wired — reuse it
		}
	}
	body := map[string]any{
		"type": flavor, "name": "Loomarr " + flavor, "uri": embyURL,
		"accessToken": token, "userId": userID, "username": "loomarr", "pathReplacements": []any{},
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := t.doJSON(ctx, http.MethodPost, "/api/media-sources", body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// ConnectLibraries enables the movie + show libraries of a source and scans each one
// it just enabled. Idempotent — an already-enabled library is left untouched (no
// re-scan on every call). Returns the number of movie/show libraries now enabled.
func (t *Tunarr) ConnectLibraries(ctx context.Context, sourceID string) (int, error) {
	var libs []srcLibrary
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources/"+sourceID+"/libraries", nil, &libs); err != nil {
		return 0, err
	}
	enabled := 0
	for _, l := range libs {
		if !isProgrammable(l.MediaType) {
			continue
		}
		enabled++
		if l.Enabled {
			continue // already enabled+scanned — don't re-trigger a scan
		}
		base := "/api/media-sources/" + sourceID + "/libraries/" + l.ID
		if err := t.doJSON(ctx, http.MethodPut, base, map[string]any{"enabled": true}, nil); err != nil {
			return enabled, err
		}
		if err := t.doJSON(ctx, http.MethodPost, base+"/scan", map[string]any{}, nil); err != nil {
			return enabled, err
		}
	}
	return enabled, nil
}

// MediaLibrariesReady reports whether Tunarr has a media-server source with an
// enabled movie/show library — the tunarr_library setup check (§6). false means a
// channel's slots would find no Tunarr program and degrade to flex/dead-air.
func (t *Tunarr) MediaLibrariesReady(ctx context.Context) (bool, error) {
	var sources []embyMediaSource
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources", nil, &sources); err != nil {
		return false, err
	}
	for _, s := range sources {
		if !isMediaServer(s.Type) {
			continue
		}
		var libs []srcLibrary
		if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources/"+s.ID+"/libraries", nil, &libs); err != nil {
			return false, err
		}
		for _, l := range libs {
			if isProgrammable(l.MediaType) && l.Enabled {
				return true, nil
			}
		}
	}
	return false, nil
}
