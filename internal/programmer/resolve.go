package programmer

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// This file resolves a media-server item id (the Emby/Jellyfin id a lineup slot
// carries) → Tunarr's own program id (§6). Tunarr's manual-programming API
// rejects the raw media-server id (FOREIGN KEY constraint) — a programming entry
// must reference a program uuid Tunarr assigned when it scanned the item. We read
// Tunarr's persisted library index and build a cached {external id → uuid} map.
//
// NB (Phase-0 finding): Tunarr's BROWSE endpoint returns ephemeral handles that
// change per request; only the persisted /media-libraries/{id}/programs index
// yields ids valid for programming.

// tunarrLibrary is one entry from GET /api/media-sources/{id}/libraries.
type tunarrLibrary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// tunarrMediaSource is one entry from GET /api/media-sources.
type tunarrMediaSource struct {
	ID   string `json:"id"`
	Type string `json:"type"` // emby | jellyfin | plex | local
}

// persistedProgram is one entry from GET /api/media-libraries/{id}/programs. The
// top-level id is the program uuid used in programming; identifiers carry the
// external (emby/jellyfin) id that matches a slot's LibraryItemID.
type persistedProgram struct {
	ID      string `json:"id"`
	Program struct {
		Type        string `json:"type"` // movie | episode | …
		Identifiers []struct {
			Type string `json:"type"` // emby | jellyfin | tmdb | …
			ID   string `json:"id"`
		} `json:"identifiers"`
	} `json:"program"`
}

// contentResolver maps external media-server item ids → Tunarr program uuids,
// caching the index per normalized Tunarr URL so a reconcile doesn't re-page 26k
// rows every time or reuse one instance's UUIDs on another. A miss triggers one
// refresh (a just-scanned item may be new); a still-missing id is reported so the
// caller can degrade that slot to flex (never dead air, §9).
type contentResolver struct {
	mu       sync.Mutex
	byExtID  map[string]string // external item id → program uuid
	loaded   bool
	cacheKey string // normalized Tunarr base URL that owns byExtID
	refresh  func(ctx context.Context) (map[string]string, error)
}

// resolve returns the Tunarr program uuid for an external item id, refreshing the
// cache once on a miss (to catch newly-scanned items). ok=false ⇒ not in Tunarr's
// index (unscanned / just-landed) → caller degrades the slot to flex.
func (r *contentResolver) resolve(ctx context.Context, cacheKey, extID string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded || r.cacheKey != cacheKey {
		if err := r.reloadLocked(ctx, cacheKey); err != nil {
			return "", false, err
		}
	}
	if uuid, ok := r.byExtID[extID]; ok {
		return uuid, true, nil
	}
	// Miss: refresh once (Tunarr may have scanned the item since we last loaded).
	if err := r.reloadLocked(ctx, cacheKey); err != nil {
		return "", false, err
	}
	uuid, ok := r.byExtID[extID]
	return uuid, ok, nil
}

func (r *contentResolver) reloadLocked(ctx context.Context, cacheKey string) error {
	m, err := r.refresh(ctx)
	if err != nil {
		return err
	}
	r.byExtID = m
	r.loaded = true
	r.cacheKey = cacheKey
	return nil
}

// buildContentIndex pages every enabled library's persisted programs and returns
// an {external item id → program uuid} map. It matches on emby/jellyfin ids (the
// same id space a slot's LibraryItemID uses — verified: Tunarr's persisted emby id
// equals the media-server item id).
func (t *Tunarr) buildContentIndex(ctx context.Context) (map[string]string, error) {
	var sources []tunarrMediaSource
	if err := t.doJSON(ctx, "GET", "/api/media-sources", nil, &sources); err != nil {
		return nil, fmt.Errorf("list media sources: %w", err)
	}
	out := make(map[string]string)
	for _, src := range sources {
		// Skip `local` sources (§10 filler): they hold commercials, not program
		// content — none of their clips carry an emby/jellyfin identifier, so they
		// contribute nothing to this index. They also don't expose the
		// `/media-sources/{id}/libraries` sub-path (it 400s for local sources; their
		// libraries come via GET /media-sources/{id}), so walking them here both
		// wastes work AND breaks the whole index build. (Caught by the live smoke:
		// adding the filler local source 400'd every program-slot resolution.)
		if strings.EqualFold(src.Type, "local") {
			continue
		}
		var libs []tunarrLibrary
		if err := t.doJSON(ctx, "GET", "/api/media-sources/"+src.ID+"/libraries", nil, &libs); err != nil {
			return nil, fmt.Errorf("list libraries for %s: %w", src.ID, err)
		}
		for _, lib := range libs {
			if !lib.Enabled {
				continue // an unscanned library has no programs to reference
			}
			var progs []persistedProgram
			// The bulk client: this endpoint returns a library's ENTIRE program list
			// with no paging (tens of MB, many seconds on a large library), so it needs
			// the long TimeoutTunarrBulk — the standard 20 s ceiling cancels it mid-pull.
			if err := t.doJSONBulk(ctx, "GET", "/api/media-libraries/"+lib.ID+"/programs", nil, &progs); err != nil {
				return nil, fmt.Errorf("list programs for library %s: %w", lib.ID, err)
			}
			for _, p := range progs {
				if p.ID == "" {
					continue
				}
				for _, id := range p.Program.Identifiers {
					if strings.EqualFold(id.Type, "emby") || strings.EqualFold(id.Type, "jellyfin") {
						out[id.ID] = p.ID
					}
				}
			}
		}
	}
	return out, nil
}
