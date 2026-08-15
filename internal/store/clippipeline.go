package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
)

// The per-clip ingest pipeline's persistence (§10 V51b, migration 00044).
//
// ⚠ **A sibling table, and the folder scan never touches it.** `clips` is a synced cache that has
// been dropped and recreated twice; this table records that ~341s of Whisper and a paid vision
// call have already been spent on a clip. Keeping it out of the cache is what makes that fact
// survive an identity change — and it makes the single-writer rule STRUCTURAL rather than a
// convention `UpsertClip`'s DO UPDATE list has to remember.

const clipPipelineSelect = `SELECT clip_hash, stage, status, progress, disposition,
	reject_reason, reject_detail, attempts, next_run, stages_json, enrolled_at, updated_at
	FROM filler_clip_pipeline`

// scanClipPipeline reads one row, decoding the ladder.
//
// ⚠ A corrupt ladder is REPORTED, never silently emptied. An empty ladder renders as "this clip
// has done nothing", which is a specific and false claim about a clip that may have been through
// every stage — the same call `ListSplitProposals` makes about corrupt segments.
func scanClipPipeline(sc scannable) (filler.ClipPipeline, error) {
	var (
		p          filler.ClipPipeline
		stage      string
		status     string
		dispo      string
		reason     string
		raw        string
		nextRun    int64
		enrolledAt int64
		updatedAt  int64
	)
	if err := sc.Scan(&p.ClipHash, &stage, &status, &p.Progress, &dispo,
		&reason, &p.RejectDetail, &p.Attempts, &nextRun, &raw, &enrolledAt, &updatedAt); err != nil {
		return filler.ClipPipeline{}, err
	}
	p.Stage = filler.StageID(stage)
	p.Status = filler.StageStatus(status)
	p.Disposition = filler.Disposition(dispo)
	p.RejectReason = filler.RejectReason(reason)
	p.NextRun = fromEpoch(nextRun)
	p.EnrolledAt = fromEpoch(enrolledAt)
	p.UpdatedAt = fromEpoch(updatedAt)
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &p.Stages); err != nil {
			return filler.ClipPipeline{}, fmt.Errorf("pipeline ladder for %s is corrupt: %w", p.ClipHash, err)
		}
	}
	return p, nil
}

// UpsertClipPipeline writes a clip's pipeline row.
//
// ⚠ **The ONLY writer of this table**, which is what keeps the state machine in one place. Unlike
// `UpsertClip` there is no omission list to maintain here: the row is authored by exactly one
// component (`filler.Pipeline`), so every column rides the update and none of them can be blanked
// by a caller that did not know about them.
func (s *sqlStore) UpsertClipPipeline(ctx context.Context, p filler.ClipPipeline) error {
	raw, err := json.Marshal(p.Stages)
	if err != nil {
		return fmt.Errorf("marshal pipeline ladder: %w", err)
	}
	if len(p.Stages) == 0 {
		raw = []byte("[]")
	}
	_, err = s.db.ExecContext(ctx, s.ph(
		`INSERT INTO filler_clip_pipeline (clip_hash, stage, status, progress, disposition,
		   reject_reason, reject_detail, attempts, next_run, stages_json, enrolled_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(clip_hash) DO UPDATE SET
		   stage=excluded.stage, status=excluded.status, progress=excluded.progress,
		   disposition=excluded.disposition, reject_reason=excluded.reject_reason,
		   reject_detail=excluded.reject_detail, attempts=excluded.attempts,
		   next_run=excluded.next_run, stages_json=excluded.stages_json,
		   updated_at=excluded.updated_at`),
		p.ClipHash, string(p.Stage), string(p.Status), p.Progress, string(p.Disposition),
		string(p.RejectReason), p.RejectDetail, p.Attempts, epoch(p.NextRun), string(raw),
		epoch(p.EnrolledAt), epoch(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert clip pipeline %s: %w", p.ClipHash, err)
	}
	return nil
}

// GetClipPipeline reads one row. Absence is not an error — an un-enrolled clip is ordinary.
func (s *sqlStore) GetClipPipeline(ctx context.Context, hash string) (filler.ClipPipeline, bool, error) {
	p, err := scanClipPipeline(s.db.QueryRowContext(ctx, s.ph(clipPipelineSelect+` WHERE clip_hash = ?`), hash))
	if err == sql.ErrNoRows {
		return filler.ClipPipeline{}, false, nil
	}
	if err != nil {
		return filler.ClipPipeline{}, false, fmt.Errorf("get clip pipeline %s: %w", hash, err)
	}
	return p, true, nil
}

// MarkPipelineFiled takes a clip OFF the belt (§10 V54) — the split sweep's step 1.
//
// ⚠ **This is what stops the sweep becoming a churn loop.** A swept composite is still marked
// `is_composite` and still enrolled, so leaving its row `running` means the split rung re-detects
// it on the very next pass — propose → partly confirm → leftovers → sweep → re-propose, burning a
// boundary scan every cycle and never converging. `ListPipelineWork` claims only `running`, so
// `filed` is the existing, one-word way to say "this reel is finished".
//
// A missing row is not an error: a clip catalogued before the pipeline existed has none, and there
// is nothing to take off a belt it was never on.
func (s *sqlStore) MarkPipelineFiled(ctx context.Context, hash string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE filler_clip_pipeline SET disposition = ?, updated_at = ? WHERE clip_hash = ?`),
		string(filler.DispositionFiled), epoch(at), hash)
	if err != nil {
		return fmt.Errorf("mark pipeline filed %s: %w", hash, err)
	}
	return nil
}

// ListPipelineWork returns the non-terminal rows that are due, oldest first.
//
// ⚠ Ordered by `next_run` then `clip_hash`. The tie-break is not cosmetic: without a total order
// the engine may return the same row on consecutive passes while another never surfaces, so one
// clip would be worked repeatedly and another would starve. `ListFillerSources` records the same
// hazard for a list an operator merely reads; here it decides what work happens.
func (s *sqlStore) ListPipelineWork(ctx context.Context, now time.Time, limit int) ([]filler.ClipPipeline, error) {
	q := clipPipelineSelect + ` WHERE disposition = ? AND next_run <= ? ORDER BY next_run, clip_hash`
	args := []any{string(filler.DispositionRunning), epoch(now)}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, s.ph(q), args...)
	if err != nil {
		return nil, fmt.Errorf("list pipeline work: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectClipPipelines(rows)
}

// ListClipPipelines reads pipeline rows for the Incoming read model.
func (s *sqlStore) ListClipPipelines(ctx context.Context, f filler.PipelineFilter) ([]filler.ClipPipeline, error) {
	q := clipPipelineSelect
	var where []string
	var args []any
	if f.ConveyorOnly {
		// ⚠ `running` OR `review` — the two halves of one belt. `review` is terminal for the
		// PIPELINE and not for the operator, so a clip sitting there is still Incoming's business.
		where = append(where, `disposition IN (?, ?)`)
		args = append(args, string(filler.DispositionRunning), string(filler.DispositionReview))
	}
	if f.RejectedOnly {
		where = append(where, `disposition = ?`)
		args = append(args, string(filler.DispositionRejected))
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	// Newest first: the rejected list is an audit feed, and what was just refused is what an
	// operator is looking for. The hash tie-break keeps paging stable on Postgres.
	q += ` ORDER BY updated_at DESC, clip_hash`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.db.QueryContext(ctx, s.ph(q), args...)
	if err != nil {
		return nil, fmt.Errorf("list clip pipelines: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectClipPipelines(rows)
}

// CountClipPipelines answers the same filtered question without materialising an audit feed.
// Incoming uses it to report an honest total while keeping the returned rows to one page.
func (s *sqlStore) CountClipPipelines(ctx context.Context, f filler.PipelineFilter) (int, error) {
	q := `SELECT COUNT(*) FROM filler_clip_pipeline`
	var where []string
	var args []any
	if f.ConveyorOnly {
		where = append(where, `disposition IN (?, ?)`)
		args = append(args, string(filler.DispositionRunning), string(filler.DispositionReview))
	}
	if f.RejectedOnly {
		where = append(where, `disposition = ?`)
		args = append(args, string(filler.DispositionRejected))
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, s.ph(q), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count clip pipelines: %w", err)
	}
	return n, nil
}

// CountIncomingConveyor counts the exact union rendered by the Incoming belt: held legacy clips
// plus running/review pipeline rows, minus reels that have their own row. Keeping this in SQL is
// what lets the response cap rows without turning its total into the page length.
func (s *sqlStore) CountIncomingConveyor(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM clips c
		WHERE NOT EXISTS (SELECT 1 FROM filler_split_proposals sp WHERE sp.clip_hash = c.hash)
		  AND ((c.removed_at = 0 AND c.held = ? AND c.is_composite = ?)
		    OR EXISTS (SELECT 1 FROM filler_clip_pipeline p
		      WHERE p.clip_hash = c.hash AND p.disposition IN (?, ?)))`),
		true, false, string(filler.DispositionRunning), string(filler.DispositionReview)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count incoming conveyor: %w", err)
	}
	return n, nil
}

// CountIncomingDecisions counts only conveyor rows the machine has handed to a person. It is the
// source for the tab badge, which must not shrink to the first response page.
func (s *sqlStore) CountIncomingDecisions(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM clips c
		WHERE c.is_composite = ?
		  AND NOT EXISTS (SELECT 1 FROM filler_split_proposals sp WHERE sp.clip_hash = c.hash)
		  AND (EXISTS (SELECT 1 FROM filler_clip_pipeline p
		         WHERE p.clip_hash = c.hash AND p.disposition = ?)
		    OR (c.removed_at = 0 AND c.held = ? AND NOT EXISTS
		       (SELECT 1 FROM filler_clip_pipeline p
		        WHERE p.clip_hash = c.hash AND p.disposition IN (?, ?))))`),
		false, string(filler.DispositionReview), true,
		string(filler.DispositionRunning), string(filler.DispositionReview)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count incoming decisions: %w", err)
	}
	return n, nil
}

// ListClipsWithoutPipeline returns catalogued clips with no pipeline row yet.
//
// ⚠ **`NOT EXISTS`, deliberately NOT a LEFT JOIN.** `clipSelect` names its columns unqualified and
// ends in `FROM clips`, and BOTH tables have an `updated_at` — so joining `filler_clip_pipeline`
// makes that column ambiguous and the query fails on both dialects. A correlated subquery adds no
// columns to resolve, so the shared select stays reusable exactly as written.
//
// (It is also not the bind-list hazard `attachTags` hit: that one builds one placeholder per clip
// and dies past Postgres's 65535-parameter cap. A subquery sends no binds at all.)
//
// ⚠ Removed clips are excluded. A tombstoned clip is not work: enrolling it would run the whole
// ladder against something deliberately taken out of rotation.
func (s *sqlStore) ListClipsWithoutPipeline(ctx context.Context, limit int) ([]filler.StoreClip, error) {
	q := clipSelect + ` WHERE clips.removed_at = 0 AND NOT EXISTS (
			SELECT 1 FROM filler_clip_pipeline p WHERE p.clip_hash = clips.hash)
		ORDER BY clips.hash`
	var args []any
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, s.ph(q), args...)
	if err != nil {
		return nil, fmt.Errorf("list clips without pipeline: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []filler.StoreClip
	for rows.Next() {
		c, err := scanClip(rows)
		if err != nil {
			return nil, fmt.Errorf("scan clip without pipeline: %w", err)
		}
		out = append(out, filler.StoreClip{Clip: c.Clip, UpdatedAt: c.UpdatedAt})
	}
	return out, rows.Err()
}

// pruneOrphanPipelines deletes pipeline rows whose clip is gone.
//
// ⚠ Called from `DeleteClipsNotIn`, which is the sync's prune — the one place clips disappear in
// bulk. It is written as "no matching clip" rather than "not in the keep set" so it stays correct
// whichever branch of the prune ran, and so a clip deleted by any other route is covered too.
//
// Errors are swallowed by the caller on purpose: the clips ARE pruned by the time this runs, and
// failing the sync over some leftover bookkeeping would turn a tidy-up into an outage. A surviving
// orphan is picked up by the next prune.
func (s *sqlStore) pruneOrphanPipelines(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM filler_clip_pipeline WHERE NOT EXISTS (
			SELECT 1 FROM clips c WHERE c.hash = filler_clip_pipeline.clip_hash)`)
	if err != nil {
		return fmt.Errorf("prune orphan pipeline rows: %w", err)
	}
	return nil
}

func collectClipPipelines(rows *sql.Rows) ([]filler.ClipPipeline, error) {
	var out []filler.ClipPipeline
	for rows.Next() {
		p, err := scanClipPipeline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
