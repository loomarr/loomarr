package clipstore

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// SetClipLanguage records what the detection job heard (§10 V40, migration 00036).
//
// ⚠ **The ONLY writer of `language`**, exactly like SetClipsRemoved owns the tombstone and
// RecordClipPlay owns the counters — and for the same reason: `UpsertClip` deliberately omits the
// column, so the folder scan cannot blank a detected language by finding the file still on disk.
// Without that omission every sync would reset the catalog to "not yet checked" and the job would
// re-detect everything on the next pass, which on the local backend is ~341s per clip under QEMU.
//
// Keyed by PATH rather than hash, because that is what the job carries and what every other
// clip-writing method here takes.
func (s *Store) SetClipLanguage(ctx context.Context, path, language string, at time.Time) error {
	_, err := s.h.ExecContext(ctx,
		s.h.Rebind(`UPDATE clips SET language = ?, updated_at = ? WHERE path = ?`),
		language, epoch(at), path)
	if err != nil {
		return fmt.Errorf("set clip language %s: %w", path, err)
	}
	return nil
}

// SetClipTranscript records what the transcribe job heard (§10 V44, migration 00038).
//
// ⚠ **The ONLY writer of `transcript`**, exactly like SetClipLanguage owns `language` and for the
// identical reason: `UpsertClip` deliberately omits the column, so the folder scan cannot blank a
// transcribed clip by finding its file still on disk. Without that omission every sync would reset
// the catalog to "not yet transcribed" and the job would re-run Whisper on everything — ~341s per
// clip under QEMU.
//
// Keyed by PATH rather than hash, because that is what the job carries and what SetClipLanguage
// beside it takes. An empty transcript is a legitimate write: it records "checked, wordless", which
// is what stops the selective job from re-visiting a silent clip forever.
func (s *Store) SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error {
	_, err := s.h.ExecContext(ctx,
		s.h.Rebind(`UPDATE clips SET
			enrichment_revision = CASE WHEN transcript <> ? THEN enrichment_revision + 1 ELSE enrichment_revision END,
			transcript = ?, updated_at = ? WHERE path = ?`),
		transcript, transcript, epoch(at), path)
	if err != nil {
		return fmt.Errorf("set clip transcript %s: %w", path, err)
	}
	return nil
}

// SetClipBrand records a GROUNDED advertiser found by the text tagger or confirmed by an operator
// (§10 V44, migration 00038).
//
// ⚠ Writes `brand` and nothing else — deliberately narrower than ApplyClipVision, which also owns
// `visible_text`. The text tagger grounds a brand in the clip's text signals (filename, sidecar, or
// the persisted transcript); the vision pass grounds one in the on-screen text. Both are legitimate
// writers of `brand`, and each writes only what it is entitled to.
//
// The caller has already applied the grounding rule (a brand is kept only when it appears
// literally in the text), so this writes what it is given. Keyed by PATH, like the transcript
// and vision writers it sits beside and unlike the hash-keyed classification update — the job carries the
// path. `UpsertClip` omits `brand` from its DO UPDATE for the same single-writer reason as every
// other job column, so a re-sync cannot blank a grounded brand and make the tagger re-derive it.
func (s *Store) SetClipBrand(ctx context.Context, path, brand string, at time.Time) error {
	_, err := s.h.ExecContext(ctx,
		s.h.Rebind(`UPDATE clips SET brand = ?, updated_at = ? WHERE path = ?`),
		brand, epoch(at), path)
	if err != nil {
		return fmt.Errorf("set clip brand %s: %w", path, err)
	}
	return nil
}

// ApplyClipVision records one semantic vision pass (§10 V44/V55): the on-screen text it saw, a
// grounded brand and era, and the taxonomy assertions supported by the frames. The facts, direct
// assertions and rollups commit together.
//
// ⚠ **The pipeline's writer of `visible_text`.** `brand` it SHARES with SetClipBrand (the text
// tagger's grounded writer) — both write `brand` and only `brand`, and this one must not be
// confused for the sole owner or a text-grounded brand would look vision-read. Same rule as
// every job column here: `UpsertClip` omits them so a re-sync cannot undo the pass. The caller has
// already applied the grounding rule (a brand/era/category survives only if `visibleText` supports
// it), so this method writes what it is given — the validation lives in the domain, not the SQL.
//
// A blank vision brand preserves a brand grounded by the text tier. Era behaves the same way. Tags
// are additive: vision can contribute evidence without erasing an operator or the text classifier.
func (s *Store) ApplyClipVision(ctx context.Context, hash, path, brand, visibleText string, era int, leaves []string, at time.Time) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("apply clip vision: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := storedb.XactLock(ctx, s.h.Dialect(), tx, taxonomySharedLockSQL); err != nil {
		return fmt.Errorf("apply clip vision: lock: %w", err)
	}
	result, err := tx.ExecContext(ctx, s.h.Rebind(
		`UPDATE clips SET
		   enrichment_revision = CASE WHEN visible_text <> ? THEN enrichment_revision + 1 ELSE enrichment_revision END,
		   brand = CASE WHEN ? <> '' THEN ? ELSE brand END,
		   visible_text = ?,
		   era = CASE WHEN ? > 0 THEN ? ELSE era END,
		   updated_at = ? WHERE hash = ? AND path = ?`),
		visibleText,
		brand, brand, visibleText,
		era, era,
		epoch(at), hash, path)
	if err != nil {
		return fmt.Errorf("apply clip vision %s: %w", path, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}

	rows, err := tx.QueryContext(ctx, s.h.Rebind(
		`SELECT taxon FROM clip_tags WHERE clip_hash = ? AND leaf = ? ORDER BY taxon`), hash, true)
	if err != nil {
		return fmt.Errorf("apply clip vision: list asserted tags: %w", err)
	}
	merged := append([]string(nil), leaves...)
	for rows.Next() {
		var leaf string
		if err := rows.Scan(&leaf); err != nil {
			_ = rows.Close()
			return fmt.Errorf("apply clip vision: scan asserted tag: %w", err)
		}
		merged = append(merged, leaf)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("apply clip vision: close asserted tags: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("apply clip vision: list asserted tags: %w", err)
	}
	if err := s.setClipTagsTx(ctx, tx, hash, merged); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearClipVisibleText forgets the on-screen text vision read (§10 V51b) — the invalidation half
// of a `Rewind` to the vision rung, whose reset ladder is what makes the rung look again.
//
// ⚠ **It clears the text vision read; it does NOT clear brand, era or category.**
// Those three are SHARED with the text tagger — `SetClipBrand` and the classification update write the same
// columns — and nothing on the row records which tier put a value there. Blanking them would
// therefore destroy a text-grounded brand, or an era an operator confirmed by hand, in order to
// re-run a tier that may not even find one. A re-read simply overwrites what it can ground, which
// is the same additive contract `ApplyClipVision` has.
//
// ⚠ A separate narrow method rather than calling `ApplyClipVision` with empty strings: that one
// writes what it is GIVEN, so the empty-argument trick works by accident today and breaks the
// first time it learns to gap-fill.
func (s *Store) ClearClipVisibleText(ctx context.Context, path string, at time.Time) error {
	_, err := s.h.ExecContext(ctx, s.h.Rebind(
		`UPDATE clips SET enrichment_revision = enrichment_revision + 1,
		 visible_text = '', updated_at = ? WHERE path = ?`),
		epoch(at), path)
	if err != nil {
		return fmt.Errorf("clear clip visible text %s: %w", path, err)
	}
	return nil
}

// SetClipComposite marks (or unmarks) a clip as a composite — a recorded break, not airable (§10
// V45). Called by intake/detection when a 16-minute file is recognised as a break, and by split
// Confirm on the PARENT (which V45 keeps and marks composite rather than deleting).
//
// ⚠ **The ONLY writer of `is_composite`**, exactly like SetClipLanguage owns `language`: UpsertClip
// omits it, so a re-sync finding the original file on disk cannot flip a confirmed composite back to
// an airable clip. Keyed by HASH — a composite is identified by its content, and the detection/
// confirm paths hold the hash.
// ClipArtworkPending is one clip whose rendered artwork has not been adopted into the image
// service yet. Paths are RELATIVE to the artwork cache directory, exactly as the columns store
// them — the store does not know where FILLER_DIR is, and resolving that here would put a
// filesystem assumption inside a query.
type ClipArtworkPending struct {
	Hash      string
	Thumbnail string
	Preview   string
}

// ListClipsPendingArtworkAdoption returns clips that HAVE rendered artwork but no image-service
// identity for it yet (§22, V52 phase 6) — the adoption job's work list.
//
// ⚠ The predicate is per-ASSET, not per-clip: a clip whose still adopted but whose animation did
// not must come back, or the hover loop would never be adopted for any clip where the two renders
// completed in different passes. `OR` rather than `AND` is the whole correctness of this query.
//
// ⚠ Tombstoned clips are excluded. Adopting artwork for a clip the operator removed would copy
// bytes into the image store purely so the GC could collect them again later.
func (s *Store) ListClipsPendingArtworkAdoption(ctx context.Context, limit int) ([]ClipArtworkPending, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(
		`SELECT hash, thumbnail, preview FROM clips
		  WHERE removed_at = 0
		    AND (   (thumbnail <> '' AND thumb_image_hash = '')
		         OR (preview   <> '' AND hover_image_hash = ''))
		  ORDER BY updated_at
		  LIMIT ?`), limit)
	if err != nil {
		return nil, fmt.Errorf("list clips pending artwork adoption: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ClipArtworkPending
	for rows.Next() {
		var p ClipArtworkPending
		if err := rows.Scan(&p.Hash, &p.Thumbnail, &p.Preview); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetClipArtworkImages records the image-service identities of a clip's still and hover loop
// (§22, V52 phase 6). The ONLY writer of those two columns — which is why they are absent from
// UpsertClip's DO UPDATE list, exactly like is_composite and the play counters.
//
// ⚠ Both are written together because they come from ONE ffmpeg pass and one ingest step, but
// either may legitimately be "" — the still can succeed while the animation fails, and a caller
// that had only one would otherwise have to read-modify-write to avoid blanking the other.
func (s *Store) SetClipArtworkImages(ctx context.Context, hash, thumbHash, hoverHash string, at time.Time) error {
	res, err := s.h.ExecContext(ctx,
		s.h.Rebind(`UPDATE clips SET thumb_image_hash = ?, hover_image_hash = ?, updated_at = ? WHERE hash = ?`),
		thumbHash, hoverHash, epoch(at), hash)
	if err != nil {
		return fmt.Errorf("set clip artwork images %s: %w", hash, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetClipComposite(ctx context.Context, hash string, composite bool, at time.Time) error {
	res, err := s.h.ExecContext(ctx,
		s.h.Rebind(`UPDATE clips SET is_composite = ?, updated_at = ? WHERE hash = ?`),
		composite, epoch(at), hash)
	if err != nil {
		return fmt.Errorf("set clip composite %s: %w", hash, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
