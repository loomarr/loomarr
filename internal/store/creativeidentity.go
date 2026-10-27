package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/clipcatalog"
)

// InferiorCopy is one copy of an ad that lost to a better copy of the same creative (#2033).
type InferiorCopy struct {
	// ClipHash is the losing copy's catalog identity.
	ClipHash string
	// SourceHash is the sparse identity of the bytes the copy was admitted from, so the same file
	// offered again is skipped at intake. Equal to ClipHash when nothing re-encoded it.
	SourceHash   string
	SurvivorHash string
	DecidedAt    time.Time
}

// ErrInferiorCopyMismatch refuses a merge that would not fold one live clip into another.
var ErrInferiorCopyMismatch = errors.New("store: inferior copy must name two different existing clips")

// MergeInferiorCopy folds a losing copy into the surviving copy of the same ad in one transaction:
// the survivor takes the loser's play count and household rotation history, its variant links,
// any copies that had lost to it, a name a person curated on it, and its channel pins and
// exclusions. The loser's catalog row and derived rows are deleted and it is remembered as an
// inferior copy. Its files are the caller's to delete; Sync then never catalogues them again.
func (s *sqlStore) MergeInferiorCopy(ctx context.Context, c InferiorCopy) error {
	if c.ClipHash == "" || c.SurvivorHash == "" || c.SourceHash == "" || c.ClipHash == c.SurvivorHash {
		return ErrInferiorCopyMismatch
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("merge inferior copy %s: %w", c.ClipHash, err)
	}
	defer func() { _ = tx.Rollback() }()

	var live int
	if err := tx.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM clips WHERE hash IN (?, ?)`),
		c.ClipHash, c.SurvivorHash).Scan(&live); err != nil {
		return fmt.Errorf("merge inferior copy %s: %w", c.ClipHash, err)
	}
	if live != 2 {
		return ErrInferiorCopyMismatch
	}
	loser, survivor := c.ClipHash, c.SurvivorHash
	curated := string(clipcatalog.NameFromCurated)
	steps := []struct {
		query string
		args  []any
	}{
		// Every airing of either copy was an airing of the ad.
		{`UPDATE clips SET
		    play_count = play_count + (SELECT play_count FROM clips WHERE hash = ?),
		    last_played_at = CASE WHEN (SELECT last_played_at FROM clips WHERE hash = ?) > last_played_at
		      THEN (SELECT last_played_at FROM clips WHERE hash = ?) ELSE last_played_at END
		  WHERE hash = ?`, []any{loser, loser, loser, survivor}},
		// A name a person wrote outranks whatever the survivor was called.
		{`UPDATE clips SET name = (SELECT name FROM clips WHERE hash = ?), name_source = ?
		  WHERE hash = ? AND name_source <> ?
		    AND EXISTS (SELECT 1 FROM clips WHERE hash = ? AND name_source = ?)`,
			[]any{loser, curated, survivor, curated, loser, curated}},
		{`INSERT INTO filler_airings (channel_id, clip_hash, aired_at, recorded_at)
		  SELECT channel_id, ?, aired_at, recorded_at FROM filler_airings WHERE clip_hash = ?
		  ON CONFLICT DO NOTHING`, []any{survivor, loser}},
		{`DELETE FROM filler_airings WHERE clip_hash = ?`, []any{loser}},
		// The loser's variants are the survivor's, kept in smaller-hash-first order.
		{`INSERT INTO filler_clip_variants (a_hash, b_hash)
		  SELECT CASE WHEN other < ? THEN other ELSE ? END, CASE WHEN other < ? THEN ? ELSE other END
		  FROM (SELECT CASE WHEN a_hash = ? THEN b_hash ELSE a_hash END AS other
		        FROM filler_clip_variants WHERE a_hash = ? OR b_hash = ?) linked
		  WHERE other <> ?
		  ON CONFLICT DO NOTHING`,
			[]any{survivor, survivor, survivor, survivor, loser, loser, loser, survivor}},
		{`DELETE FROM filler_clip_variants WHERE a_hash = ? OR b_hash = ?`, []any{loser, loser}},
		{`UPDATE filler_inferior_copies SET survivor_hash = ? WHERE survivor_hash = ?`, []any{survivor, loser}},
		{`INSERT INTO filler_inferior_copies (clip_hash, source_hash, survivor_hash, decided_at)
		  VALUES (?, ?, ?, ?)
		  ON CONFLICT(clip_hash) DO UPDATE SET source_hash = excluded.source_hash,
		    survivor_hash = excluded.survivor_hash, decided_at = excluded.decided_at`,
			[]any{loser, c.SourceHash, survivor, epoch(c.DecidedAt)}},
		{`DELETE FROM clip_tags WHERE clip_hash = ?`, []any{loser}},
		{`DELETE FROM filler_clip_fingerprints WHERE clip_hash = ?`, []any{loser}},
		{`DELETE FROM filler_clip_pipeline WHERE clip_hash = ?`, []any{loser}},
		{`DELETE FROM image_refs WHERE owner_kind = ? AND owner_id = ?`, []any{"clip", loser}},
		{`DELETE FROM clips WHERE hash = ?`, []any{loser}},
	}
	for _, step := range steps {
		if _, err := tx.ExecContext(ctx, s.ph(step.query), step.args...); err != nil {
			return fmt.Errorf("merge inferior copy %s: %w", loser, err)
		}
	}
	if err := s.rekeyChannelClipRefs(ctx, tx, loser, survivor); err != nil {
		return fmt.Errorf("merge inferior copy %s channel references: %w", loser, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("merge inferior copy %s: %w", loser, err)
	}
	return nil
}

// LinkClipVariants records that two clips are separate ads built from the same footage.
func (s *sqlStore) LinkClipVariants(ctx context.Context, a, b string) error {
	if a == "" || b == "" || a == b {
		return fmt.Errorf("link clip variants: two different clips are required")
	}
	if b < a {
		a, b = b, a
	}
	if _, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO filler_clip_variants (a_hash, b_hash) VALUES (?, ?) ON CONFLICT DO NOTHING`), a, b); err != nil {
		return fmt.Errorf("link clip variants %s/%s: %w", a, b, err)
	}
	return nil
}

// ListClipVariants maps every linked clip to the clips it is linked with.
func (s *sqlStore) ListClipVariants(ctx context.Context) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a_hash, b_hash FROM filler_clip_variants`)
	if err != nil {
		return nil, fmt.Errorf("list clip variants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]string{}
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, fmt.Errorf("scan clip variants: %w", err)
		}
		out[a] = append(out[a], b)
		out[b] = append(out[b], a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list clip variants: %w", err)
	}
	return out, nil
}

// InferiorCopyHashes is every catalog and source identity of a known inferior copy. Intake skips
// a file whose identity is in it, and Sync never catalogues one, so a copy that lost stays gone
// even when its files outlive the merge.
func (s *sqlStore) InferiorCopyHashes(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT clip_hash, source_hash FROM filler_inferior_copies`)
	if err != nil {
		return nil, fmt.Errorf("list inferior copies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var clipHash, sourceHash string
		if err := rows.Scan(&clipHash, &sourceHash); err != nil {
			return nil, fmt.Errorf("scan inferior copies: %w", err)
		}
		out[clipHash], out[sourceHash] = true, true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list inferior copies: %w", err)
	}
	return out, nil
}
