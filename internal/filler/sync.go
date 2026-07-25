package filler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// This file is the catalog sync (§10, revised by §9.1): loomarr scans FILLER_DIR
// itself and probes each clip with ffprobe. Path + name + DURATION come from that
// scan; loomarr owns the match metadata (era/audience/category) too, and PRESERVES
// it across syncs so a re-sync never clobbers hand-edited or AI-assigned tags.
// `/v1/filler/sync` triggers it; a periodic sync runs alongside the reconciler
// (FILLER_SYNC_EVERY, §15).
//
// It previously synced FROM the Tunarr `local` filler source, with clip identity
// being the Tunarr program uuid. See Clip for why that could not serve internal
// playout (a uuid is not a playable input) and why the dependency ran the wrong
// way (no Tunarr ⇒ no catalog ⇒ no commercials, on a service §9.1 makes optional).

// RawClip is one clip as discovered by a scan, before catalog metadata is merged.
type RawClip struct {
	// Path is the identity: the clip's location relative to FILLER_DIR.
	Path string
	// TunarrProgramID is set only when the clip was ALSO seen through Tunarr's local
	// source, so Tunarr-backed channels can still build filler-lists. Empty on an
	// install with no Tunarr, which is a supported configuration, not a degraded one.
	TunarrProgramID string
	Name            string
	DurationMs      int64
	Kind            Kind
	Era             int // initial era from filename; 0 if none
}

// FillerSource discovers the clips in FILLER_DIR.
//
// Implemented by DirSource (the local scan, §9.1) and satisfied in tests by a double. It was
// previously implemented by the Tunarr client — EnsureLocalSource registered the drop-folder
// as a Tunarr `local` media source and ListLocalClips read back what Tunarr had scanned.
//
// EnsureLocalSource is retained because Tunarr-backed channels still need that registration
// (their filler-lists reference Tunarr program ids). It is now BEST-EFFORT: an install with no
// Tunarr fails it harmlessly and still gets a full catalog from the local scan, which is the
// whole point of the change.
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
	// Register the drop-folder with Tunarr, when there is one (idempotent, §10).
	//
	// BEST-EFFORT since §9.1, and the distinction is the whole point of that change: this
	// registration exists so TUNARR-backed channels can reference clips in a filler-list, and
	// it has nothing to do with discovering the files — Loomarr scans FILLER_DIR itself. An
	// install with no Tunarr, or one whose Tunarr is momentarily down, must still get a full
	// catalog; failing here would restore exactly the dependency §9.1 removed.
	if err := s.source.EnsureLocalSource(ctx, s.dir); err != nil && s.log != nil {
		s.log.Warn("filler: could not register the drop-folder with Tunarr; "+
			"scanning locally anyway (Tunarr-backed channels may lack filler until it returns)",
			"dir", s.dir, "err", err)
	}
	raw, err := s.source.ListLocalClips(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list filler clips: %w", err)
	}

	res := SyncResult{Total: len(raw)}
	keep := make([]string, 0, len(raw))
	for _, rc := range raw {
		keep = append(keep, rc.Path)

		existing, found, err := s.store.GetClip(ctx, rc.Path)
		if err != nil {
			return res, fmt.Errorf("get clip %s: %w", rc.Path, err)
		}

		merged := StoreClip{UpdatedAt: s.now()}
		merged.Path = rc.Path
		// Scan-owned fields (always taken fresh — the filesystem is source of truth).
		merged.Name = rc.Name
		merged.DurationMs = rc.DurationMs
		merged.Kind = rc.Kind
		// Carry the Tunarr uuid when the scan found one. Taken fresh rather than preserved:
		// a re-registered Tunarr local source mints new program ids, and a stale uuid would
		// build a filler-list referencing programs Tunarr no longer has.
		merged.TunarrProgramID = rc.TunarrProgramID
		if found {
			// PRESERVE loomarr-owned match tags across syncs (§10) — never clobber a
			// hand-edited or AI-assigned era/audience/category.
			merged.Era = existing.Era
			merged.Audience = existing.Audience
			merged.Category = existing.Category
			merged.AITagged = existing.AITagged
			merged.Rating = existing.Rating
			merged.Source = existing.Source
			if existing.TunarrProgramID != "" && rc.TunarrProgramID == "" {
				// Keep a known uuid when THIS scan could not see Tunarr (it is offline, or
				// this install has none). Losing it would silently strip filler from a
				// Tunarr-backed channel on the next reconcile.
				merged.TunarrProgramID = existing.TunarrProgramID
			}
			if serverFieldsUnchanged(existing.Clip, merged.Clip) {
				continue // idempotent: nothing changed, skip the write
			}
			res.Updated++
		} else {
			// New clip: seed era from the filename hint; leave audience/category
			// untagged for AI/manual tagging.
			merged.Era = rc.Era
			merged.Source = "filler-dir"
			res.Added++
		}
		if err := s.store.UpsertClip(ctx, merged); err != nil {
			return res, fmt.Errorf("upsert clip %s: %w", rc.Path, err)
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
