package tunarr

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// This file wires the media server as *Tunarr's* media source (§6): so Tunarr can
// stream the underlying files and index them into its program table. Loomarr does
// this for the operator during setup (POST /v1/setup/tunarr-connect) using its
// existing admin token — Tunarr accepts the admin X-Emby-Token as the source access
// token, so there is no separate Emby user login and Loomarr stores no extra
// credential. Enumerate-first + idempotent, like the Live TV wiring and filler.

// sourceName is the name of the source Loomarr creates; it marks the source as Loomarr's.
func sourceName(flavor string) string { return "Loomarr " + flavor }

type embyMediaSource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
// existing source for embyURL rather than creating a duplicate, and re-pushes token to
// the source Loomarr created so a rotated library token reaches Tunarr. A same-URL
// source Loomarr did not create is reused untouched. token/userID are the admin API
// key + the admin user's id (Tunarr uses them as the source access token).
func (t *Tunarr) EnsureEmbySource(ctx context.Context, flavor, embyURL, token, userID string) (string, error) {
	ctx, _ = t.operation(ctx)
	var existing []embyMediaSource
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources", nil, &existing); err != nil {
		return "", err
	}
	want := strings.TrimRight(embyURL, "/")
	name := sourceName(flavor)
	body := map[string]any{
		"type": flavor, "name": name, "uri": embyURL,
		"accessToken": token, "userId": userID, "username": "loomarr", "pathReplacements": []any{},
	}
	foreignID := ""
	for _, s := range existing {
		if s.Type != flavor || strings.TrimRight(s.URI, "/") != want {
			continue
		}
		if s.Name == name {
			if err := t.pushToken(ctx, s.ID, embyURL, token); err != nil {
				return "", err
			}
			return s.ID, nil
		}
		if foreignID == "" {
			foreignID = s.ID
		}
	}
	if foreignID != "" {
		return foreignID, nil // someone else's source for this server — reuse, never modify
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := t.doJSON(ctx, http.MethodPost, "/api/media-sources", body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// pushToken sets the access token (and the uri, when it differs) on Loomarr's source.
// Tunarr's PUT replaces the whole record and trashes content under path mappings that
// are no longer present, so the stored source is read back and written unchanged apart
// from those fields; the operator's path replacements and every other field survive.
// Tunarr redacts the stored token on read, so a rotation can't be detected by
// comparing — the PUT is idempotent and keeps Tunarr's copy equal to the library token.
func (t *Tunarr) pushToken(ctx context.Context, id, embyURL, token string) error {
	var current map[string]any
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources/"+id, nil, &current); err != nil {
		return fmt.Errorf("read Tunarr media source %s: %w", id, err)
	}
	current["accessToken"] = token
	if uri, _ := current["uri"].(string); strings.TrimRight(uri, "/") != strings.TrimRight(embyURL, "/") {
		current["uri"] = embyURL
	}
	if err := t.doJSON(ctx, http.MethodPut, "/api/media-sources/"+id, current, nil); err != nil {
		return fmt.Errorf("update Tunarr media source %s: %w", id, err)
	}
	return nil
}

// ConnectLibraries enables the movie + show libraries of a source and scans each one
// it just enabled. Idempotent — an already-enabled library is left untouched (no
// re-scan on every call). Returns the number of movie/show libraries now enabled.
func (t *Tunarr) ConnectLibraries(ctx context.Context, sourceID string) (int, error) {
	ctx, _ = t.operation(ctx)
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
	ctx, _ = t.operation(ctx)
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
