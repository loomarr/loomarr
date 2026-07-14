package filler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// This file is the catalog sync (§10): loomarr syncs its clip catalog FROM the
// Tunarr `local` filler source. Program uuids + names + DURATION come from
// Tunarr's scan (the core never downloads or probes media); loomarr owns only the
// match metadata (era/audience/category), which it PRESERVES across syncs so a
// re-sync never clobbers hand-edited or AI-assigned tags. `/v1/filler/sync`
// triggers it (ensuring the Tunarr local source on first run); a periodic sync
// runs alongside the reconciler (FILLER_SYNC_EVERY, §15).

// RawClip is one clip as read from Tunarr's local source (mirrors
// programmer.LocalClip; declared here so the sync doesn't import programmer —
// FillerSource provides the reader, filler consumes it via that port).
type RawClip struct {
	TunarrProgramID string
	Name            string
	DurationMs      int64
	Kind            Kind
	Era             int // initial era from filename; 0 if none
}

// FillerSource reads the Tunarr `local` filler source (implemented by the Tunarr
// client via a main.go adapter). EnsureLocalSource registers + scans the drop-
// folder on first run (idempotent); ListLocalClips reads the scanned programs.
type FillerSource interface {
	EnsureLocalSource(ctx context.Context, dir string) error
	ListLocalClips(ctx context.Context) ([]RawClip, error)
}

// Store is the slice of the store the sync needs.
type Store interface {
	UpsertClip(ctx context.Context, c StoreClip) error
	GetClip(ctx context.Context, id string) (StoreClip, bool, error)
	DeleteClipsNotIn(ctx context.Context, keepIDs []string) (int, error)
}

// StoreClip is the persistence view the sync round-trips (mirrors store.Clip;
// declared here so filler doesn't import store — the adapter in main bridges them).
type StoreClip struct {
	Clip
	UpdatedAt time.Time
}

// Syncer reconciles the clip catalog against the Tunarr `local` filler source.
type Syncer struct {
	source FillerSource
	store  Store
	dir    string // FILLER_DIR (§15) — the drop-folder registered as a Tunarr local source
	log    *slog.Logger
	now    func() time.Time
}

// NewSyncer builds a catalog syncer. dir is the drop-folder path (FILLER_DIR, §15)
// registered with Tunarr as a `local` media source.
func NewSyncer(source FillerSource, store Store, dir string, now func() time.Time, log *slog.Logger) *Syncer {
	if now == nil {
		now = time.Now
	}
	return &Syncer{source: source, store: store, dir: dir, log: log, now: now}
}

// SyncResult reports what a sync did (for the API + logs).
type SyncResult struct {
	Total   int // clips in the Tunarr local filler source
	Added   int // new clips
	Updated int // existing clips whose server-derived fields changed
	Pruned  int // clips removed (gone from the source)
}

// Sync ensures the Tunarr local source exists, then reconciles the catalog (§10):
//   - upsert every scanned clip, PRESERVING loomarr-owned tags (era/audience/
//     category/ai) on clips we already know — Tunarr only owns id/name/duration/
//     kind-hint;
//   - prune clips no longer in the source (identity = Tunarr program uuid).
//
// Duration always comes from Tunarr's scan. Idempotent: a no-change re-sync makes
// no tag edits.
func (s *Syncer) Sync(ctx context.Context) (SyncResult, error) {
	if s.dir == "" {
		return SyncResult{}, fmt.Errorf("filler sync: no FILLER_DIR configured")
	}
	// Ensure the Tunarr local filler source exists + is scanned (idempotent, §10).
	if err := s.source.EnsureLocalSource(ctx, s.dir); err != nil {
		return SyncResult{}, fmt.Errorf("ensure tunarr local filler source: %w", err)
	}
	raw, err := s.source.ListLocalClips(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list tunarr filler clips: %w", err)
	}

	res := SyncResult{Total: len(raw)}
	keep := make([]string, 0, len(raw))
	for _, rc := range raw {
		keep = append(keep, rc.TunarrProgramID)

		existing, found, err := s.store.GetClip(ctx, rc.TunarrProgramID)
		if err != nil {
			return res, fmt.Errorf("get clip %s: %w", rc.TunarrProgramID, err)
		}

		merged := StoreClip{UpdatedAt: s.now()}
		merged.TunarrProgramID = rc.TunarrProgramID
		// Tunarr-owned fields (always taken fresh — Tunarr's scan is source of truth).
		merged.Name = rc.Name
		merged.DurationMs = rc.DurationMs
		merged.Kind = rc.Kind
		if found {
			// PRESERVE loomarr-owned match tags across syncs (§10) — never clobber a
			// hand-edited or AI-assigned era/audience/category.
			merged.Era = existing.Era
			merged.Audience = existing.Audience
			merged.Category = existing.Category
			merged.AITagged = existing.AITagged
			merged.Rating = existing.Rating
			merged.Source = existing.Source
			if serverFieldsUnchanged(existing.Clip, merged.Clip) {
				continue // idempotent: nothing changed, skip the write
			}
			res.Updated++
		} else {
			// New clip: seed era from the filename hint; leave audience/category
			// untagged for AI/manual tagging.
			merged.Era = rc.Era
			merged.Source = "tunarr-local"
			res.Added++
		}
		if err := s.store.UpsertClip(ctx, merged); err != nil {
			return res, fmt.Errorf("upsert clip %s: %w", rc.TunarrProgramID, err)
		}
	}

	pruned, err := s.store.DeleteClipsNotIn(ctx, keep)
	if err != nil {
		return res, fmt.Errorf("prune clips: %w", err)
	}
	res.Pruned = pruned
	if s.log != nil {
		s.log.Info("filler catalog synced", "total", res.Total, "added", res.Added, "updated", res.Updated, "pruned", res.Pruned)
	}
	return res, nil
}

// serverFieldsUnchanged reports whether the Tunarr-owned fields match (so a
// re-sync is a no-op write). Tags aren't compared — they're loomarr-owned and
// preserved, not synced.
func serverFieldsUnchanged(a, b Clip) bool {
	return a.Name == b.Name && a.DurationMs == b.DurationMs && a.Kind == b.Kind
}
