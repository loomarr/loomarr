package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// The persisted split proposal (§10, V34 — migration 00025). Detection runs
// minutes per file and REVIEW IS NOT OPTIONAL (detection quality is a property
// of the source, measured 69–100%), so a proposal must survive a restart and a
// delayed review. It is deliberately NOT part of the clip catalog: pod matching
// never sees it, and segments become clips only through Confirm.

const splitProposalSelect = `SELECT id, clip_hash, segments_json, created_at FROM filler_split_proposals`

// splitProposalDocument evolves the original bare segment array without a coordination-column
// migration. Detection checkpoints are implementation state, not independently queryable data;
// keeping them in the proposal's one authored/read document preserves that ownership.
type splitProposalDocument struct {
	Version   int                            `json:"version"`
	Segments  []filler.SplitSegment          `json:"segments,omitempty"`
	Detection *filler.SplitDetectionProgress `json:"detection,omitempty"`
	Spawned   []string                       `json:"spawned,omitempty"`
}

func marshalSplitProposal(p filler.SplitProposal) ([]byte, error) {
	return json.Marshal(splitProposalDocument{Version: 2, Segments: p.Segments, Detection: p.Detection, Spawned: p.Spawned})
}

func unmarshalSplitProposal(raw string, p *filler.SplitProposal) error {
	trimmed := bytes.TrimSpace([]byte(raw))
	if len(trimmed) > 0 && trimmed[0] == '[' {
		// V34–V54 stored the segment array directly. Absence of detector state means those rows
		// are complete proposals, so upgrades need no migration or queue rewrite.
		return json.Unmarshal(trimmed, &p.Segments)
	}
	var doc splitProposalDocument
	if err := json.Unmarshal(trimmed, &doc); err != nil {
		return err
	}
	p.Segments, p.Detection, p.Spawned = doc.Segments, doc.Detection, doc.Spawned
	return nil
}

// UpsertSplitProposal writes a proposal. ONE proposal per compilation clip:
// re-running detection replaces the pending one (clip_hash is UNIQUE), because
// two competing cut-lists for one file is a review bug, not a choice.
func (s *sqlStore) UpsertSplitProposal(ctx context.Context, p filler.SplitProposal) error {
	raw, err := marshalSplitProposal(p)
	if err != nil {
		return fmt.Errorf("marshal split proposal document: %w", err)
	}
	_, err = s.db.ExecContext(ctx, s.ph(
		`INSERT INTO filler_split_proposals (id, clip_hash, segments_json, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(clip_hash) DO UPDATE SET
		   id=excluded.id, segments_json=excluded.segments_json, created_at=excluded.created_at`),
		p.ID, p.ClipHash, string(raw), epoch(p.CreatedAt))
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
		Scan(&p.ID, &p.ClipHash, &raw, &createdAt)
	if err == sql.ErrNoRows {
		return filler.SplitProposal{}, ErrNotFound
	}
	if err != nil {
		return filler.SplitProposal{}, fmt.Errorf("get split proposal %s: %w", id, err)
	}
	if err := unmarshalSplitProposal(raw, &p); err != nil {
		return filler.SplitProposal{}, fmt.Errorf("split proposal %s document corrupt: %w", id, err)
	}
	p.CreatedAt = fromEpoch(createdAt)
	return p, nil
}

// SweepableProposal is one reel the split sweep may retire (§10 V54): its remaining cuts have sat
// unreviewed past the window, and it has already given up clips.
type SweepableProposal struct {
	ProposalID string
	ClipHash   string
	ClipPath   string
	Segments   int
}

// ListSweepableSplitProposals finds proposals created before `before` whose compilation has ALREADY
// PRODUCED CLIPS.
//
// ⚠ **The `EXISTS` clause is the safety rule, not an optimisation.** A reel that yielded nothing is
// the operator's only copy of that content, and reaping it would destroy material Loomarr never
// managed to use — the sweep would be deleting downloads on a timer for no gain. A reel whose cuts
// are already in the catalog has given up what it had; its recording is spent.
//
// ⚠ Already-reaped rows are excluded, so the sweep is idempotent: a second run finds nothing to do
// rather than re-deleting a file that is gone and re-stamping a timestamp.
func (s *sqlStore) ListSweepableSplitProposals(ctx context.Context, before time.Time) ([]SweepableProposal, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT p.id, p.clip_hash, c.path, p.segments_json
		   FROM filler_split_proposals p
		   JOIN clips c ON c.hash = p.clip_hash
		  WHERE p.created_at < ?
		    AND c.reaped_at IS NULL
		    AND EXISTS (SELECT 1 FROM clips k WHERE k.parent_hash = p.clip_hash)
		  ORDER BY p.created_at, p.id`), epoch(before))
	if err != nil {
		return nil, fmt.Errorf("list sweepable split proposals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SweepableProposal
	for rows.Next() {
		var (
			sp  SweepableProposal
			raw string
		)
		if err := rows.Scan(&sp.ProposalID, &sp.ClipHash, &sp.ClipPath, &raw); err != nil {
			return nil, fmt.Errorf("scan sweepable split proposal: %w", err)
		}
		var segs []filler.SplitSegment
		// A corrupt blob must not stop the sweep: the count is for the log line, not the decision.
		_ = json.Unmarshal([]byte(raw), &segs)
		sp.Segments = len(segs)
		out = append(out, sp)
	}
	return out, rows.Err()
}

// MarkClipReaped records that a composite's recording has been reclaimed (§10 V54). The row stays;
// only the bytes are gone.
func (s *sqlStore) MarkClipReaped(ctx context.Context, hash string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE clips SET reaped_at = ?, updated_at = ? WHERE hash = ?`), epoch(at), epoch(at), hash)
	if err != nil {
		return fmt.Errorf("mark clip %s reaped: %w", hash, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark clip %s reaped: %w", hash, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// pruneOrphanSplitProposals deletes proposals whose compilation is gone.
//
// ⚠ The same rule, and the same reason, as `pruneOrphanPipelines`: `filler_split_proposals` is a
// sibling of `clips` with NO foreign key — deliberately, so it survives a `clips` rebuild — and
// the price of that independence is that nothing else will ever clean it up.
//
// ⚠ **An orphan proposal is not inert.** Incoming renders one as a "compilation to review" titled
// with a raw 64-character hash (the name falls back to the clip identity when `GetClip` misses),
// carrying a Review-cuts button that opens a review of a file that no longer exists. Measured
// 2026-08-11: deleting every clip file and running filler-sync pruned `clips` to 0 and left **48**
// such rows, which then dominated the tab.
//
// Written as "no matching clip" rather than "not in the keep set" so it stays correct whichever
// branch of the prune ran, and so a clip deleted by any other route is covered too. Errors are
// swallowed by the caller for the same reason as the pipeline prune: the clips ARE gone by then,
// and failing the sync over leftover bookkeeping turns a tidy-up into an outage.
func (s *sqlStore) pruneOrphanSplitProposals(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM filler_split_proposals WHERE NOT EXISTS (
			SELECT 1 FROM clips c WHERE c.hash = filler_split_proposals.clip_hash)`)
	if err != nil {
		return fmt.Errorf("prune orphan split proposals: %w", err)
	}
	return nil
}

// UpdateSplitProposal replaces the document of an EXISTING proposal (§10 V54 — split-time
// grounding and partial-confirm output accumulate across passes, so a pass writes back what it learned).
//
// ⚠ **Deliberately NOT `UpsertSplitProposal`.** That one is `INSERT … ON CONFLICT(clip_hash)`, so
// a grounding write landing after `Confirm` consumed the proposal would RESURRECT it: a pending
// review for a reel that has already been cut, pointing at a composite whose segments are in the
// catalog. The read-modify-write here spans minutes of vision calls, which is ample time for that
// race. `ErrNotFound` when the row is gone is the entire point.
//
// ⚠ It does not touch `created_at`. `ListSplitProposals` orders by it, so writing it would let a
// reel jump the review queue merely for having been grounded.
func (s *sqlStore) UpdateSplitProposal(ctx context.Context, p filler.SplitProposal) error {
	raw, err := marshalSplitProposal(p)
	if err != nil {
		return fmt.Errorf("marshal split proposal document: %w", err)
	}
	res, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE filler_split_proposals SET segments_json = ? WHERE id = ?`), string(raw), p.ID)
	if err != nil {
		return fmt.Errorf("update split proposal %s: %w", p.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update split proposal %s: %w", p.ID, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
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

// ListSplitProposals returns every pending proposal, OLDEST FIRST — the Filler page's Incoming
// tab (V35), where the compilation that has been waiting longest is the one to review next.
//
// Ordering is explicit rather than left to the engine: an unordered list reshuffles between
// reads on Postgres, and a review queue whose rows move under the pointer is its own bug.
func (s *sqlStore) ListSplitProposals(ctx context.Context) ([]filler.SplitProposal, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(splitProposalSelect+` ORDER BY created_at, id`))
	if err != nil {
		return nil, fmt.Errorf("list split proposals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return collectReadyOrAllSplitProposals(rows, 0, false)
}

// ListReadySplitProposals returns at most limit proposals that have finished detection. It scans
// checkpoint documents without retaining them, so a large detection backlog cannot become an
// equally large Incoming payload.
func (s *sqlStore) ListReadySplitProposals(ctx context.Context, limit int) ([]filler.SplitProposal, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(splitProposalSelect+` ORDER BY created_at, id`))
	if err != nil {
		return nil, fmt.Errorf("list ready split proposals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectReadyOrAllSplitProposals(rows, limit, true)
}

// CountReadySplitProposals counts reviewable reels without retaining their JSON documents.
func (s *sqlStore) CountReadySplitProposals(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(splitProposalSelect+` ORDER BY created_at, id`))
	if err != nil {
		return 0, fmt.Errorf("count ready split proposals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		var (
			p         filler.SplitProposal
			raw       string
			createdAt int64
		)
		if err := rows.Scan(&p.ID, &p.ClipHash, &raw, &createdAt); err != nil {
			return 0, fmt.Errorf("scan split proposal: %w", err)
		}
		if err := unmarshalSplitProposal(raw, &p); err != nil {
			return 0, fmt.Errorf("split proposal %s document corrupt: %w", p.ID, err)
		}
		if p.Ready() {
			n++
		}
	}
	return n, rows.Err()
}

func collectReadyOrAllSplitProposals(rows *sql.Rows, limit int, readyOnly bool) ([]filler.SplitProposal, error) {
	var out []filler.SplitProposal
	for rows.Next() {
		var (
			p         filler.SplitProposal
			raw       string
			createdAt int64
		)
		if err := rows.Scan(&p.ID, &p.ClipHash, &raw, &createdAt); err != nil {
			return nil, fmt.Errorf("scan split proposal: %w", err)
		}
		// Reported rather than silently skipped: a proposal whose segments will not decode is
		// a compilation the operator can never review, and dropping it from the list makes it
		// invisible instead of fixable.
		if err := unmarshalSplitProposal(raw, &p); err != nil {
			return nil, fmt.Errorf("split proposal %s document corrupt: %w", p.ID, err)
		}
		p.CreatedAt = fromEpoch(createdAt)
		if readyOnly && !p.Ready() {
			continue
		}
		out = append(out, p)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}
