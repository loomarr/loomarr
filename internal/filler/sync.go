package filler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// This file is the catalog sync (§10): loomarr syncs its clip catalog FROM the
// media server's filler library. Item ids + names + DURATION come from the server
// (the core never downloads or probes media); loomarr owns only the match
// metadata (era/audience/category), which it PRESERVES across syncs so a re-sync
// never clobbers hand-edited or AI-assigned tags. `/v1/filler/sync` triggers it;
// a periodic sync runs alongside the reconciler (FILLER_SYNC_EVERY, §15).

// RawClip is one clip as read from the media server (mirrors library.FillerClip;
// declared here so the sync doesn't import library, keeping the dependency one-way
// — library provides the reader, filler consumes it via the Lister port).
type RawClip struct {
	LibraryItemID string
	Name          string
	DurationMs    int64
	Kind          Kind
	Era           int // initial era from filename; 0 if none
}

// Lister reads the media server's filler library (implemented by library.Client).
type Lister interface {
	ListFillerClips(ctx context.Context, fillerLibraryID string) ([]RawClip, error)
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

// Syncer reconciles the clip catalog against the media server's filler library.
type Syncer struct {
	lister    Lister
	store     Store
	libraryID string
	log       *slog.Logger
	now       func() time.Time
}

// NewSyncer builds a catalog syncer. libraryID is the media-server filler library
// id (FILLER_LIBRARY, §15).
func NewSyncer(lister Lister, store Store, libraryID string, now func() time.Time, log *slog.Logger) *Syncer {
	if now == nil {
		now = time.Now
	}
	return &Syncer{lister: lister, store: store, libraryID: libraryID, log: log, now: now}
}

// SyncResult reports what a sync did (for the API + logs).
type SyncResult struct {
	Total   int // clips in the media server's filler library
	Added   int // new clips
	Updated int // existing clips whose server-derived fields changed
	Pruned  int // clips removed (gone from the media server)
}

// Sync reads the filler library and reconciles the catalog (§10):
//   - upsert every server clip, PRESERVING loomarr-owned tags (era/audience/
//     category/ai) on clips we already know — the server only owns id/name/
//     duration/kind-hint;
//   - prune clips no longer in the library (identity = server item id, §4).
//
// Duration always comes from the server. Idempotent: a no-change re-sync makes no
// tag edits.
func (s *Syncer) Sync(ctx context.Context) (SyncResult, error) {
	if s.libraryID == "" {
		return SyncResult{}, fmt.Errorf("filler sync: no FILLER_LIBRARY configured")
	}
	raw, err := s.lister.ListFillerClips(ctx, s.libraryID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list filler library: %w", err)
	}

	res := SyncResult{Total: len(raw)}
	keep := make([]string, 0, len(raw))
	for _, rc := range raw {
		keep = append(keep, rc.LibraryItemID)

		existing, found, err := s.store.GetClip(ctx, rc.LibraryItemID)
		if err != nil {
			return res, fmt.Errorf("get clip %s: %w", rc.LibraryItemID, err)
		}

		merged := StoreClip{UpdatedAt: s.now()}
		merged.LibraryItemID = rc.LibraryItemID
		// Server-owned fields (always taken from the server — it's source of truth).
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
			merged.Source = "media-server"
			res.Added++
		}
		if err := s.store.UpsertClip(ctx, merged); err != nil {
			return res, fmt.Errorf("upsert clip %s: %w", rc.LibraryItemID, err)
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

// serverFieldsUnchanged reports whether the server-owned fields match (so a
// re-sync is a no-op write). Tags aren't compared — they're loomarr-owned and
// preserved, not synced.
func serverFieldsUnchanged(a, b Clip) bool {
	return a.Name == b.Name && a.DurationMs == b.DurationMs && a.Kind == b.Kind
}
