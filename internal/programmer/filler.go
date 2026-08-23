package programmer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// This file is the Tunarr side of the §10 filler redesign: filler is a pure
// Loomarr↔Tunarr concern (the media server is out of the filler path). The drop-
// folder is registered as a Tunarr `local` media source; Tunarr scans it and
// assigns each clip a program uuid + duration; Loomarr reads that catalog and
// builds a per-channel filler-list Tunarr plays into the flex gaps the scheduler
// leaves between programs. All idempotent, enumerate-first (mirrors the Live TV
// connector, §6). The core never probes media — Tunarr does the scanning (§10).

// --- wire types (Tunarr 1.3.8; contract pinned in api/vendor/tunarr-openapi.json
//     + the live-prototype captures, memory loomarr-filler-redesign) ---

// localMediaSourceReq is the `local` branch of POST /api/media-sources. mediaType
// is "other_videos" for commercials (NOT "movies"); paths is the drop-folder(s).
type localMediaSourceReq struct {
	Name             string     `json:"name"`
	Type             string     `json:"type"`      // "local"
	MediaType        string     `json:"mediaType"` // "other_videos"
	Paths            []string   `json:"paths"`
	PathReplacements []struct{} `json:"pathReplacements"` // always [] for us
}

// mediaSourceDetail is GET /api/media-sources/{id}: a local source auto-creates a
// library per path, each carrying its own id + externalKey (= the path).
type mediaSourceDetail struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Libraries []struct {
		ID          string `json:"id"`
		ExternalKey string `json:"externalKey"`
	} `json:"libraries"`
}

// mediaSourceSummary is one entry of GET /api/media-sources with the fields the
// enumerate-first check needs (type + the local source's configured paths).
type mediaSourceSummary struct {
	ID    string   `json:"id"`
	Type  string   `json:"type"`
	Name  string   `json:"name"`
	Paths []string `json:"paths"`
}

// localProgram is one entry of GET /api/media-libraries/{id}/programs for a local
// source. The top-level id is the program uuid (the clip identity); the program
// object carries duration + title and must be echoed back verbatim to the filler-
// list API (a minimal {id,duration} is rejected). We keep the raw program object.
type localProgram struct {
	ID      string          `json:"id"`
	Program json.RawMessage `json:"program"`
}

// LocalClip is one scanned filler clip as read from Tunarr's local source (§10):
// the program uuid (identity), display name, duration, and the raw program object
// (echoed to the filler-list API). The Syncer maps this into the clip catalog.
type LocalClip struct {
	ProgramID  string
	Name       string
	DurationMs int64
	Program    json.RawMessage
}

// programMeta is the minimal shape decoded from a localProgram.Program to pull
// out the display fields; the raw bytes are kept separately for the filler-list.
type programMeta struct {
	Title    string `json:"title"`
	Duration int64  `json:"duration"` // milliseconds (Tunarr scan)
}

// EnsureLocalSourceResult reports what EnsureLocalSource did, so a caller/UI can
// distinguish "created the source" from "already there" (mirrors setup.ConnectResult).
type EnsureLocalSourceResult struct {
	SourceID    string
	LibraryIDs  []string
	SourceAdded bool
	Scanned     bool
}

// AlreadyWired reports the idempotent no-op case (source existed, nothing added).
func (r EnsureLocalSourceResult) AlreadyWired() bool { return !r.SourceAdded }

// EnsureLocalFillerSource registers the drop-folder as a Tunarr `local` media
// source if absent, then triggers a per-library scan (§10). Enumerate-first: it
// matches an existing local source by a configured drop path, so a second call is
// a no-op create (scanning is always safe to re-run). Returns the source + its
// library ids (the catalog read enumerates those libraries' programs).
func (t *Tunarr) EnsureLocalFillerSource(ctx context.Context, dir string) (EnsureLocalSourceResult, error) {
	ctx, _ = t.operation(ctx)
	var res EnsureLocalSourceResult
	if dir == "" {
		return res, fmt.Errorf("ensure local filler source: empty dir")
	}

	var sources []mediaSourceSummary
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources", nil, &sources); err != nil {
		return res, fmt.Errorf("enumerate media sources: %w", err)
	}
	for _, s := range sources {
		if s.Type == "local" && containsPath(s.Paths, dir) {
			res.SourceID = s.ID // already registered
			break
		}
	}

	if res.SourceID == "" {
		req := localMediaSourceReq{
			Name:             "Loomarr Filler",
			Type:             "local",
			MediaType:        "other_videos",
			Paths:            []string{dir},
			PathReplacements: []struct{}{},
		}
		var created struct {
			ID string `json:"id"`
		}
		if err := t.doJSON(ctx, http.MethodPost, "/api/media-sources", req, &created); err != nil {
			return res, fmt.Errorf("create local filler source: %w", err)
		}
		if created.ID == "" {
			return res, fmt.Errorf("create local filler source: server returned no id")
		}
		res.SourceID = created.ID
		res.SourceAdded = true
	}

	// Enumerate the source's auto-created libraries and scan each. Scanning the
	// SOURCE 400s — the scan is per-library (memory loomarr-filler-redesign).
	var detail mediaSourceDetail
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources/"+res.SourceID, nil, &detail); err != nil {
		return res, fmt.Errorf("read filler source %s: %w", res.SourceID, err)
	}
	for _, lib := range detail.Libraries {
		res.LibraryIDs = append(res.LibraryIDs, lib.ID)
		// Scan is best-effort and non-fatal: a 202 means accepted, but a "scan
		// already in progress" (4xx) or a transient 5xx must NOT abort the sync —
		// listing already-scanned clips still works, and the scan retries next tick.
		// Only a transport error (doStatus err) is worth surfacing, and even then we
		// log via the returned error at the sync layer rather than failing the list.
		status, _, err := t.doStatus(ctx, t.http, http.MethodPost,
			"/api/media-sources/"+res.SourceID+"/libraries/"+lib.ID+"/scan", nil, nil)
		if err == nil && status >= 200 && status < 300 {
			res.Scanned = true
		}
		// A non-2xx or transient error is tolerated: the clips already indexed are
		// still listable, and a later sync re-triggers the scan (idempotent).
	}
	return res, nil
}

// ListLocalFillerClips reads the scanned clips from a source's libraries (§10).
// Identity = the Tunarr program uuid; duration + title come from Tunarr's scan.
func (t *Tunarr) ListLocalFillerClips(ctx context.Context, sourceID string) ([]LocalClip, error) {
	ctx, _ = t.operation(ctx)
	var detail mediaSourceDetail
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources/"+sourceID, nil, &detail); err != nil {
		return nil, fmt.Errorf("read filler source %s: %w", sourceID, err)
	}
	var out []LocalClip
	for _, lib := range detail.Libraries {
		var progs []localProgram
		if err := t.doJSON(ctx, http.MethodGet, "/api/media-libraries/"+lib.ID+"/programs", nil, &progs); err != nil {
			return nil, fmt.Errorf("list programs for filler library %s: %w", lib.ID, err)
		}
		for _, p := range progs {
			var meta programMeta
			_ = json.Unmarshal(p.Program, &meta) // best-effort display fields
			out = append(out, LocalClip{
				ProgramID:  p.ID,
				Name:       meta.Title,
				DurationMs: meta.Duration,
				Program:    p.Program,
			})
		}
	}
	return out, nil
}

// ListLocalFillerClipsAll reads the scanned clips across ALL `local` media sources
// (§10) — the sync-facing read that doesn't require the caller to track the source
// id. Returns an empty slice when no local source exists yet (before the first
// EnsureLocalFillerSource).
func (t *Tunarr) ListLocalFillerClipsAll(ctx context.Context) ([]LocalClip, error) {
	ctx, _ = t.operation(ctx)
	var sources []mediaSourceSummary
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources", nil, &sources); err != nil {
		return nil, fmt.Errorf("enumerate media sources: %w", err)
	}
	var out []LocalClip
	for _, s := range sources {
		if s.Type != "local" {
			continue
		}
		clips, err := t.ListLocalFillerClips(ctx, s.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, clips...)
	}
	return out, nil
}

// --- filler-list build/attach (reconcile-facing) ---

// fillerListReq is POST /api/filler-lists. Each program echoes the FULL program
// object from /programs (a minimal {id,duration} is rejected by Tunarr).
type fillerListReq struct {
	Name     string              `json:"name"`
	Programs []fillerListProgram `json:"programs"`
}

type fillerListProgram struct {
	Type     string          `json:"type"` // "content"
	ID       string          `json:"id"`
	Duration int64           `json:"duration"`
	Program  json.RawMessage `json:"program"`
}

// fillerListSummary is one entry of GET /api/filler-lists (for enumerate-first).
type fillerListSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ContentCount int    `json:"contentCount"`
}

// fillerListProgramEntry is one entry of GET /api/filler-lists/{id}/programs; only
// the program id is needed to compare the attached set against the desired set.
type fillerListProgramEntry struct {
	ID string `json:"id"`
}

// fillerCollection is the channel-side reference to one filler list. Tunarr
// requires all three fields when the channel is updated.
type fillerCollection struct {
	ID              string `json:"id"`
	Weight          int    `json:"weight"`
	CooldownSeconds int    `json:"cooldownSeconds"`
}

// existingProgramIDs returns the program-id set currently in a filler list, so the
// idempotency check compares CONTENTS (not just count) — a re-tagged catalog that
// yields a different but equal-sized pool must still update (count alone would miss
// it). A read error is surfaced so the caller rebuilds rather than wrongly no-op.
func (t *Tunarr) existingProgramIDs(ctx context.Context, listID string) ([]string, error) {
	var entries []fillerListProgramEntry
	if err := t.doJSON(ctx, http.MethodGet, "/api/filler-lists/"+listID+"/programs", nil, &entries); err != nil {
		return nil, err
	}
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids, nil
}

// sameProgramSet reports whether two program-id slices are equal in order and
// value (the pool is deterministic, so order is stable for an unchanged catalog).
func sameProgramSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// fillerListName is the stable per-channel name of Loomarr's owned filler list, so
// a re-run finds and updates the same list rather than creating duplicates.
func fillerListName(tunarrID string) string { return "loomarr:" + tunarrID }

// EnsureFillerList builds/updates the channel's Loomarr-owned filler list from the
// matched clip pool (program uuids) and attaches it to the channel (§10). It is
// enumerate-first and internally idempotent: unchanged list contents are not
// rewritten, and an unchanged channel attachment is also a no-op. A policy-only
// change still updates the attachment so live weight/cooldown settings apply on
// the next operation. Each uuid is resolved to its full program object from the
// cached local-source index (Tunarr rejects a minimal {id,duration}); an uuid
// absent from the index (not yet scanned) is skipped rather than failing the whole
// list.
func (t *Tunarr) EnsureFillerList(ctx context.Context, tunarrID string, programIDs []string) error {
	ctx, _ = t.operation(ctx)
	name := fillerListName(tunarrID)

	var existing []fillerListSummary
	if err := t.doJSON(ctx, http.MethodGet, "/api/filler-lists", nil, &existing); err != nil {
		return fmt.Errorf("enumerate filler lists: %w", err)
	}
	var found *fillerListSummary
	for i := range existing {
		if existing[i].Name == name {
			found = &existing[i]
			break
		}
	}

	// Resolve the desired uuids to their full program objects (Tunarr rejects a
	// minimal {id,duration}) BEFORE any idempotency decision, so the comparison is
	// against what we'd actually push — an uuid not yet scanned is dropped here, so
	// the "resolved set" is the real desired set. A load error aborts (don't wrongly
	// treat a transient failure as "no change").
	var resolved []fillerListProgram
	if len(programIDs) > 0 {
		index, err := t.fillerProgramIndex(ctx)
		if err != nil {
			return fmt.Errorf("load filler program index: %w", err)
		}
		resolved = make([]fillerListProgram, 0, len(programIDs))
		for _, id := range programIDs {
			prog, ok := index[id]
			if !ok {
				continue // not yet scanned by Tunarr → skip (resolves on a later sync)
			}
			resolved = append(resolved, fillerListProgram{
				Type: "content", ID: id, Duration: prog.DurationMs, Program: prog.Program,
			})
		}
	}
	desiredIDs := make([]string, len(resolved))
	for i, p := range resolved {
		desiredIDs[i] = p.ID
	}

	// Empty desired set → ensure no list is attached (detach path): if one exists,
	// clear it; otherwise nothing to do. The channel falls back to flex / bumper
	// card. This also covers "programs given but none resolvable yet" (§10): rather
	// than leaving a stale list of now-invalid ids attached, detach it.
	if len(desiredIDs) == 0 {
		if found == nil {
			return nil
		}
		if err := t.doJSON(ctx, http.MethodDelete, "/api/filler-lists/"+found.ID, nil, nil); err != nil {
			return fmt.Errorf("clear filler list %s: %w", found.ID, err)
		}
		return t.attachFillerList(ctx, tunarrID, "")
	}

	// Idempotency: compare the desired program set against the list's ACTUAL
	// contents (not just count) — a re-tagged catalog can yield a different but
	// equal-sized pool, which count alone would miss. On a read error we fall
	// through and rebuild (safe: an idempotent PUT), never wrongly no-op.
	if found != nil {
		if current, err := t.existingProgramIDs(ctx, found.ID); err == nil && sameProgramSet(current, desiredIDs) {
			// The list contents are already correct, but its channel-side weight and
			// cooldown are independent live settings. Reconcile that attachment so a
			// policy save applies on this operation without rewriting the list itself.
			return t.attachFillerList(ctx, tunarrID, found.ID)
		}
	}

	body := fillerListReq{Name: name, Programs: resolved}

	var listID string
	if found != nil {
		// Update in place (PUT) so the id stays stable for the channel attach.
		if err := t.doJSON(ctx, http.MethodPut, "/api/filler-lists/"+found.ID, body, nil); err != nil {
			return fmt.Errorf("update filler list %s: %w", found.ID, err)
		}
		listID = found.ID
	} else {
		var created struct {
			ID string `json:"id"`
		}
		if err := t.doJSON(ctx, http.MethodPost, "/api/filler-lists", body, &created); err != nil {
			return fmt.Errorf("create filler list %q: %w", name, err)
		}
		if created.ID == "" {
			return fmt.Errorf("create filler list %q: server returned no id", name)
		}
		listID = created.ID
	}
	return t.attachFillerList(ctx, tunarrID, listID)
}

// fillerProgram is the resolved program object + duration for a filler-list entry.
type fillerProgram struct {
	DurationMs int64
	Program    json.RawMessage
}

// fillerProgramIndex builds a uuid → program-object map from Loomarr's local
// filler source (§10). It enumerates the `local` media sources, reads each of
// their libraries' programs, and keys by the program uuid. Rebuilt per call (a
// reconcile touches it at most once per channel); a cache keyed on scan-version
// is a future refinement (§20). If no local source exists yet, returns an empty
// index (the caller skips → flex).
func (t *Tunarr) fillerProgramIndex(ctx context.Context) (map[string]fillerProgram, error) {
	var sources []mediaSourceSummary
	if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources", nil, &sources); err != nil {
		return nil, fmt.Errorf("enumerate media sources: %w", err)
	}
	index := map[string]fillerProgram{}
	for _, s := range sources {
		if s.Type != "local" {
			continue
		}
		var detail mediaSourceDetail
		if err := t.doJSON(ctx, http.MethodGet, "/api/media-sources/"+s.ID, nil, &detail); err != nil {
			return nil, fmt.Errorf("read local source %s: %w", s.ID, err)
		}
		for _, lib := range detail.Libraries {
			var progs []localProgram
			if err := t.doJSON(ctx, http.MethodGet, "/api/media-libraries/"+lib.ID+"/programs", nil, &progs); err != nil {
				return nil, fmt.Errorf("list programs for library %s: %w", lib.ID, err)
			}
			for _, p := range progs {
				var meta programMeta
				_ = json.Unmarshal(p.Program, &meta)
				index[p.ID] = fillerProgram{DurationMs: meta.Duration, Program: p.Program}
			}
		}
	}
	return index, nil
}

// attachFillerList sets the channel's fillerCollections to reference listID (or
// clears it when listID is ""). It reads the current channel first so the PUT
// doesn't clobber other fields (§10 risk note).
func (t *Tunarr) attachFillerList(ctx context.Context, tunarrID, listID string) error {
	var ch tunarrChannel
	if err := t.doJSON(ctx, http.MethodGet, "/api/channels/"+tunarrID, nil, &ch); err != nil {
		return fmt.Errorf("read channel %s for filler attach: %w", tunarrID, err)
	}
	// An empty desired attachment detaches every Loomarr-managed filler list.
	desired := []fillerCollection{}
	desiredRepeatCooldown := ch.FillerRepeatCooldown
	if listID != "" {
		// Tunarr's channel fillerCollections entry REQUIRES id + weight +
		// cooldownSeconds (all three; a bare {id} → 400 "Bad Request"). With one managed
		// list its LIST-selection cooldown is deliberately zero. Clip repetition is Tunarr's
		// CHANNEL-level fillerRepeatCooldown (milliseconds); wiring the user setting into the
		// list field made the whole list unavailable without protecting individual clips.
		cfg := t.configFor(ctx)
		weight := cfg.FillerWeight
		if weight <= 0 {
			weight = 1
		}
		cooldownSeconds := cfg.FillerCooldownSeconds
		if cooldownSeconds < 0 {
			cooldownSeconds = 1800
		}
		desired = append(desired, fillerCollection{
			ID: listID, Weight: weight, CooldownSeconds: 0,
		})
		desiredRepeatCooldown = int64(cooldownSeconds) * 1000
	}
	// Preserve the established detach write after deleting a list: Tunarr may
	// still hold the deleted reference even when its GET response omits it. For a
	// live attachment, however, an exact match is a true no-op.
	if listID != "" && sameFillerCollections(ch.FillerColls, desired) && ch.FillerRepeatCooldown == desiredRepeatCooldown {
		return nil
	}
	ch.FillerColls = make([]any, len(desired))
	for i := range desired {
		ch.FillerColls[i] = desired[i]
	}
	ch.FillerRepeatCooldown = desiredRepeatCooldown
	if err := t.doJSON(ctx, http.MethodPut, "/api/channels/"+tunarrID, ch, nil); err != nil {
		return fmt.Errorf("attach filler list to channel %s: %w", tunarrID, err)
	}
	return nil
}

// sameFillerCollections compares only the required attachment contract. GET
// decodes tunarrChannel.FillerColls into generic maps, so round-trip through the
// typed wire shape before comparing it with the desired policy.
func sameFillerCollections(raw []any, desired []fillerCollection) bool {
	if len(raw) != len(desired) {
		return false
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var current []fillerCollection
	if err := json.Unmarshal(blob, &current); err != nil || len(current) != len(desired) {
		return false
	}
	for i := range current {
		if current[i] != desired[i] {
			return false
		}
	}
	return true
}

// containsPath reports whether dir is among a source's configured paths.
func containsPath(paths []string, dir string) bool {
	for _, p := range paths {
		if p == dir {
			return true
		}
	}
	return false
}
