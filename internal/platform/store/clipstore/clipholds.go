package clipstore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// HoldClips sends clips to review. Ordinary callers deliberately receive no boolean that could
// spell `held=false`; only the applied-admission transaction may release a playable clip.
func (s *Store) HoldClips(ctx context.Context, paths []string, at time.Time) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(paths))
	args := make([]any, 0, len(paths)+1)
	args = append(args, epoch(at))
	for i, p := range paths {
		placeholders[i] = "?"
		args = append(args, p)
	}
	q := `UPDATE clips SET held = TRUE, updated_at = ? WHERE path IN (` +
		strings.Join(placeholders, ",") + `)`
	res, err := s.h.ExecContext(ctx, s.h.Rebind(q), args...)
	if err != nil {
		return 0, fmt.Errorf("hold clips: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count held clips: %w", err)
	}
	return int(n), nil
}

// ReleaseCompositeHolds exposes confirmed compilation lineage without creating a publication
// capability. The `is_composite = TRUE` predicate is load-bearing: composite rows are excluded
// from pod selection independently of `held`, while a non-composite must use applied admission.
func (s *Store) ReleaseCompositeHolds(ctx context.Context, paths []string, at time.Time) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(paths))
	args := make([]any, 0, len(paths)+2)
	args = append(args, epoch(at))
	for i, p := range paths {
		placeholders[i] = "?"
		args = append(args, p)
	}
	args = append(args, true)
	q := `UPDATE clips SET held = FALSE, updated_at = ? WHERE path IN (` +
		strings.Join(placeholders, ",") + `) AND is_composite = ?`
	res, err := s.h.ExecContext(ctx, s.h.Rebind(q), args...)
	if err != nil {
		return 0, fmt.Errorf("release composite holds: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count released composite holds: %w", err)
	}
	return int(n), nil
}

// SetClipsRemoved tombstones (or restores) clips by path — the Catalog tab's "Remove from
// catalog" (V35).
//
// ⚠ This, ReplaceSplitChildren, RetryClipPipeline, and the applied-admission transaction are the
// ONLY writers of `removed_at`.
// Recovery is the narrow exception because restoring the tombstone and requeueing its failed row
// must be atomic. `UpsertClip` deliberately omits the column, so the next scan
// cannot resurrect a removed clip merely by finding its file still on disk.
//
// ⚠ It does NOT touch the file. Nothing in Loomarr deletes an operator's media — the button says
// remove from the CATALOG, and the file stays where they put it.
//
// Returns the number of rows affected; unknown paths are silently skipped, because a bulk action
// over a list the operator selected minutes ago races a re-scan and failing the whole batch for
// one stale row would be worse than removing the rest.
func (s *Store) SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error) {
	if len(paths) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(paths))
	args := make([]any, 0, len(paths)+1)
	args = append(args, epoch(at))
	for i, p := range paths {
		placeholders[i] = "?"
		args = append(args, p)
	}
	// ⚠ A reaped row has no file (a swept reel, a retired defective copy), so restoring it would
	// put a missing file on air. It stays removed; removing one again is harmless.
	q := `UPDATE clips SET removed_at = ? WHERE path IN (` + strings.Join(placeholders, ",") + `)`
	if at.IsZero() {
		q += ` AND reaped_at IS NULL`
	}
	res, err := s.h.ExecContext(ctx, s.h.Rebind(q), args...)
	if err != nil {
		return 0, fmt.Errorf("set clips removed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("set clips removed: %w", err)
	}
	return int(n), nil
}

// ReplaceSplitChildren completes a re-split by making keepHashes the active generation for one
// composite. Old children are TOMBSTONED, never deleted; their bytes and metadata remain available
// to restore. Any clip explicitly pinned by a channel joins the keep set, because a detector
// improvement must not silently invalidate an operator override.
//
// The policy reads and clip writes share one transaction. Postgres locks channel rows while their
// pins are decoded, so a concurrent channel CAS cannot add a pin in the gap between the read and
// retirement. Current-generation hashes are restored too: a cut retired by an earlier generation
// may become byte-identical to a newly accepted cut, and UpsertClip intentionally preserves its
// tombstone on an ordinary scan.
func (s *Store) ReplaceSplitChildren(ctx context.Context, parentHash string, keepHashes []string, at time.Time) (int, error) {
	if parentHash == "" {
		return 0, errors.New("replace split children: parent hash is required")
	}
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("replace split children %s: %w", parentHash, err)
	}
	defer func() { _ = tx.Rollback() }()
	n, err := s.replaceSplitChildrenTx(ctx, tx, parentHash, keepHashes, at)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("replace split children %s: %w", parentHash, err)
	}
	return n, nil
}

func (s *Store) replaceSplitChildrenTx(ctx context.Context, tx storedb.Querier, parentHash string, keepHashes []string, at time.Time) (int, error) {
	keep, err := s.pinnedClipsTx(ctx, tx)
	if err != nil {
		return 0, fmt.Errorf("replace split children %s: %w", parentHash, err)
	}
	for _, hash := range keepHashes {
		if hash != "" {
			keep[hash] = struct{}{}
		}
	}

	hashes := make([]string, 0, len(keep))
	for hash := range keep {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	placeholders := make([]string, len(hashes))
	for i := range hashes {
		placeholders[i] = "?"
	}

	if len(hashes) > 0 {
		args := make([]any, 0, len(hashes)+3)
		args = append(args, epoch(at), parentHash)
		for _, hash := range hashes {
			args = append(args, hash)
		}
		q := `UPDATE clips SET removed_at = 0, updated_at = ? WHERE parent_hash = ? AND hash IN (` + strings.Join(placeholders, ",") + `)`
		if _, err := tx.ExecContext(ctx, s.h.Rebind(q), args...); err != nil {
			return 0, fmt.Errorf("replace split children %s restore generation: %w", parentHash, err)
		}
	}

	args := []any{epoch(at), epoch(at), parentHash}
	q := `UPDATE clips SET removed_at = ?, updated_at = ? WHERE parent_hash = ? AND removed_at = 0`
	if len(hashes) > 0 {
		q += ` AND hash NOT IN (` + strings.Join(placeholders, ",") + `)`
		for _, hash := range hashes {
			args = append(args, hash)
		}
	}
	res, err := tx.ExecContext(ctx, s.h.Rebind(q), args...)
	if err != nil {
		return 0, fmt.Errorf("replace split children %s retire old generation: %w", parentHash, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("replace split children %s count retired: %w", parentHash, err)
	}
	return int(n), nil
}
