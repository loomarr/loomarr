package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/mantonx/loomarr/internal/filler"
)

// The persisted split proposal (§10, V34 — migration 00025). Detection runs
// minutes per file and REVIEW IS NOT OPTIONAL (detection quality is a property
// of the source, measured 69–100%), so a proposal must survive a restart and a
// delayed review. It is deliberately NOT part of the clip catalog: pod matching
// never sees it, and segments become clips only through Confirm.

const splitProposalSelect = `SELECT id, clip_path, segments_json, created_at FROM filler_split_proposals`

// UpsertSplitProposal writes a proposal. ONE proposal per compilation clip:
// re-running detection replaces the pending one (clip_path is UNIQUE), because
// two competing cut-lists for one file is a review bug, not a choice.
func (s *sqlStore) UpsertSplitProposal(ctx context.Context, p filler.SplitProposal) error {
	raw, err := json.Marshal(p.Segments)
	if err != nil {
		return fmt.Errorf("marshal split segments: %w", err)
	}
	_, err = s.db.ExecContext(ctx, s.ph(
		`INSERT INTO filler_split_proposals (id, clip_path, segments_json, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(clip_path) DO UPDATE SET
		   id=excluded.id, segments_json=excluded.segments_json, created_at=excluded.created_at`),
		p.ID, p.ClipPath, string(raw), epoch(p.CreatedAt))
	if err != nil {
		return fmt.Errorf("upsert split proposal %s: %w", p.ID, err)
	}
	return nil
}

// GetSplitProposal reads one proposal by id — the review surface's source of
// truth on SSE reconnect (§7).
func (s *sqlStore) GetSplitProposal(ctx context.Context, id string) (filler.SplitProposal, error) {
	var (
		p         filler.SplitProposal
		raw       string
		createdAt int64
	)
	err := s.db.QueryRowContext(ctx, s.ph(splitProposalSelect+` WHERE id = ?`), id).
		Scan(&p.ID, &p.ClipPath, &raw, &createdAt)
	if err == sql.ErrNoRows {
		return filler.SplitProposal{}, ErrNotFound
	}
	if err != nil {
		return filler.SplitProposal{}, fmt.Errorf("get split proposal %s: %w", id, err)
	}
	if err := json.Unmarshal([]byte(raw), &p.Segments); err != nil {
		return filler.SplitProposal{}, fmt.Errorf("split proposal %s segments corrupt: %w", id, err)
	}
	p.CreatedAt = fromEpoch(createdAt)
	return p, nil
}

// DeleteSplitProposal removes a proposal — after confirm, and on reject.
// ErrNotFound for an unknown id, so a caller cannot believe it recorded something.
func (s *sqlStore) DeleteSplitProposal(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM filler_split_proposals WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete split proposal %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete split proposal %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteClip removes ONE clip by identity. Used by split confirm: the
// compilation's identity is a path that after the cut means twenty clips, not
// one (§10 V34). ErrNotFound for an unknown path.
func (s *sqlStore) DeleteClip(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM clips WHERE path = ?`), id)
	if err != nil {
		return fmt.Errorf("delete clip %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete clip %s: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
