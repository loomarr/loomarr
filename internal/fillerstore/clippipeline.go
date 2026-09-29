package fillerstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// The per-clip ingest pipeline's persistence (§10 V51b, migration 00044).
//
// ⚠ **A sibling table, and the folder scan never touches it.** `clips` is a synced cache that has
// been dropped and recreated twice; this table records that ~341s of Whisper and a paid vision
// call have already been spent on a clip. Keeping it out of the cache is what makes that fact
// survive an identity change — and it makes the single-writer rule STRUCTURAL rather than a
// convention `UpsertClip`'s DO UPDATE list has to remember.

const clipPipelineSelect = `SELECT clip_hash, acquisition_id, stage, status, progress, disposition,
	reject_reason, reject_detail, attempts, force_run, next_run,
	preparation_attempt, preparation_started_at, preparation_start_reason, preparation_progress,
	stage_queued_at, stage_started_at, stages_json, enrolled_at, updated_at
	FROM filler_clip_pipeline`

// scanClipPipeline reads one row, decoding the ladder.
//
// ⚠ A corrupt ladder is REPORTED, never silently emptied. An empty ladder renders as "this clip
// has done nothing", which is a specific and false claim about a clip that may have been through
// every stage — the same call `ListSplitProposals` makes about corrupt segments.
func scanClipPipeline(sc scannable) (filler.ClipPipeline, error) {
	var (
		p                      filler.ClipPipeline
		stage                  string
		status                 string
		dispo                  string
		reason                 string
		raw                    string
		nextRun                int64
		preparationStartedAt   int64
		preparationStartReason string
		stageQueuedAt          int64
		stageStartedAt         int64
		enrolledAt             int64
		updatedAt              int64
	)
	if err := sc.Scan(&p.ClipHash, &p.AcquisitionID, &stage, &status, &p.Progress, &dispo,
		&reason, &p.RejectDetail, &p.Attempts, &p.ForceRun, &nextRun,
		&p.PreparationAttempt, &preparationStartedAt, &preparationStartReason, &p.PreparationProgress,
		&stageQueuedAt, &stageStartedAt, &raw, &enrolledAt, &updatedAt); err != nil {
		return filler.ClipPipeline{}, err
	}
	p.Stage = filler.StageID(stage)
	p.Status = filler.StageStatus(status)
	p.Disposition = filler.Disposition(dispo)
	p.RejectReason = filler.RejectReason(reason)
	p.NextRun = fromEpoch(nextRun)
	p.PreparationStartedAt = fromEpoch(preparationStartedAt)
	p.PreparationStartReason = filler.PreparationStartReason(preparationStartReason)
	p.StageQueuedAt = fromEpoch(stageQueuedAt)
	p.StageStartedAt = fromEpoch(stageStartedAt)
	p.EnrolledAt = fromEpoch(enrolledAt)
	p.UpdatedAt = fromEpoch(updatedAt)
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &p.Stages); err != nil {
			return filler.ClipPipeline{}, fmt.Errorf("pipeline ladder for %s is corrupt: %w", p.ClipHash, err)
		}
	}
	return p, nil
}

// UpsertClipPipeline writes a clip's ordinary runner transition.
//
// ⚠ **This file is the ONLY writer of this table**, which is what keeps the state machine in one place. Unlike
// `UpsertClip` there is no omission list to maintain here: the row is authored by exactly one
// component (`filler.Pipeline`), so every column rides the update and none of them can be blanked
// by a caller that did not know about them.
func (s *sqlStore) UpsertClipPipeline(ctx context.Context, p filler.ClipPipeline) error {
	if err := s.writeClipPipeline(ctx, s.db, p); err != nil {
		return fmt.Errorf("upsert clip pipeline %s: %w", p.ClipHash, err)
	}
	return nil
}

type pipelineExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *sqlStore) writeClipPipeline(ctx context.Context, exec pipelineExecer, p filler.ClipPipeline) error {
	raw, err := json.Marshal(p.Stages)
	if err != nil {
		return fmt.Errorf("marshal pipeline ladder: %w", err)
	}
	if len(p.Stages) == 0 {
		raw = []byte("[]")
	}
	_, err = exec.ExecContext(ctx, s.ph(
		`INSERT INTO filler_clip_pipeline (clip_hash, acquisition_id, stage, status, progress, disposition,
		   reject_reason, reject_detail, attempts, force_run, next_run,
		   preparation_attempt, preparation_started_at, preparation_start_reason, preparation_progress,
		   stage_queued_at, stage_started_at, stages_json, enrolled_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(clip_hash) DO UPDATE SET
		   acquisition_id=excluded.acquisition_id,
		   stage=excluded.stage, status=excluded.status, progress=excluded.progress,
		   disposition=excluded.disposition, reject_reason=excluded.reject_reason,
		   reject_detail=excluded.reject_detail, attempts=excluded.attempts,
		   force_run=excluded.force_run,
		   preparation_attempt=excluded.preparation_attempt,
		   preparation_started_at=excluded.preparation_started_at,
		   preparation_start_reason=excluded.preparation_start_reason,
		   preparation_progress=excluded.preparation_progress,
		   stage_queued_at=excluded.stage_queued_at,
		   stage_started_at=excluded.stage_started_at,
		   next_run=excluded.next_run, stages_json=excluded.stages_json,
		   updated_at=excluded.updated_at`),
		p.ClipHash, p.AcquisitionID, string(p.Stage), string(p.Status), p.Progress, string(p.Disposition),
		string(p.RejectReason), p.RejectDetail, p.Attempts, p.ForceRun, epoch(p.NextRun),
		p.PreparationAttempt, epoch(p.PreparationStartedAt), string(p.PreparationStartReason), p.PreparationProgress,
		epoch(p.StageQueuedAt), epoch(p.StageStartedAt), string(raw), epoch(p.EnrolledAt), epoch(p.UpdatedAt))
	if err != nil {
		return err
	}
	return nil
}

// settlePipelineTx settles the clip's pipeline row at `to`, done, if its disposition is one of
// from. It reports whether the row changed. An admission decision settles the row in its own
// transaction.
func (s *sqlStore) settlePipelineTx(ctx context.Context, tx store.Tx, hash string, from []filler.Disposition, to filler.Disposition, at time.Time) (bool, error) {
	marks := make([]string, len(from))
	args := make([]any, 0, len(from)+3)
	args = append(args, string(to), epoch(at))
	for i, disposition := range from {
		marks[i] = "?"
		args = append(args, string(disposition))
	}
	args = append(args, hash)
	return affectedOne(tx.ExecContext(ctx, s.ph(`UPDATE filler_clip_pipeline
		SET disposition = ?, status = 'done', next_run = 0, updated_at = ?
		WHERE disposition IN (`+strings.Join(marks, ",")+`) AND clip_hash = ?`), args...))
}

// advancePipelineTx moves the clip's pipeline row from one disposition to the next, leaving its
// status and schedule alone, and reports whether it moved. Split confirmation completes the reel's
// row and starts its children's.
func (s *sqlStore) advancePipelineTx(ctx context.Context, tx store.Tx, hash string, from, to filler.Disposition, at time.Time) (bool, error) {
	return affectedOne(tx.ExecContext(ctx, s.ph(`UPDATE filler_clip_pipeline SET disposition = ?, updated_at = ?
		WHERE clip_hash = ? AND disposition = ?`),
		string(to), epoch(at), hash, string(from)))
}

// affectedOne reports whether a statement changed exactly one row.
func affectedOne(result sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect affected rows: %w", err)
	}
	return affected == 1, nil
}

// RetryClipPipeline commits the cross-table recovery boundary. A terminal execution failure has
// been tombstoned; it must become present and queued together, and it remains held until the
// downstream score stage admits it. If invalidation failed, the caller never reaches this method.
func (s *sqlStore) RetryClipPipeline(ctx context.Context, failed, p filler.ClipPipeline, restore bool) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin clip pipeline retry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if restore {
		restored, err := s.db.Clips(tx).RestoreForRetry(ctx, p.ClipHash, p.UpdatedAt)
		if err != nil {
			return fmt.Errorf("hold restored retry clip %s: %w", p.ClipHash, err)
		}
		if !restored {
			return fmt.Errorf("restore retry clip %s: clip is not in the catalog", p.ClipHash)
		}
	}
	raw, err := json.Marshal(p.Stages)
	if err != nil {
		return fmt.Errorf("marshal retry pipeline ladder: %w", err)
	}
	if len(p.Stages) == 0 {
		raw = []byte("[]")
	}
	res, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_clip_pipeline SET
		stage = ?, status = ?, progress = ?, disposition = ?, reject_reason = ?, reject_detail = ?,
		attempts = ?, force_run = ?, next_run = ?, preparation_attempt = ?, preparation_started_at = ?,
		preparation_start_reason = ?, preparation_progress = ?, stage_queued_at = ?, stage_started_at = ?,
		stages_json = ?, updated_at = ?
		WHERE clip_hash = ? AND stage = ? AND status = ? AND disposition = ?
		  AND reject_reason = ? AND attempts = ? AND updated_at = ?`),
		string(p.Stage), string(p.Status), p.Progress, string(p.Disposition), string(p.RejectReason),
		p.RejectDetail, p.Attempts, p.ForceRun, epoch(p.NextRun), p.PreparationAttempt,
		epoch(p.PreparationStartedAt), string(p.PreparationStartReason), p.PreparationProgress,
		epoch(p.StageQueuedAt), epoch(p.StageStartedAt), string(raw), epoch(p.UpdatedAt),
		failed.ClipHash, string(failed.Stage), string(failed.Status), string(failed.Disposition),
		string(failed.RejectReason), failed.Attempts, epoch(failed.UpdatedAt))
	if err != nil {
		return fmt.Errorf("retry clip pipeline %s: %w", p.ClipHash, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("count retry clip pipeline %s: %w", p.ClipHash, err)
	} else if n != 1 {
		return fmt.Errorf("%w: clip %s changed before retry", filler.ErrPipelineNotRetryable, p.ClipHash)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit clip pipeline retry %s: %w", p.ClipHash, err)
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

// MarkPipelineComplete takes a composite OFF the belt (§10 V54) — the split sweep's step 1.
//
// ⚠ **This is what stops the sweep becoming a churn loop.** A swept composite is still marked
// `is_composite` and still enrolled, so leaving its row `running` means the split rung re-detects
// it on the very next pass — propose → partly confirm → leftovers → sweep → re-propose, burning a
// boundary scan every cycle and never converging. `ListPipelineWork` claims only `running`, so
// `complete` says the reel finished processing without claiming it is Ready or playable.
//
// A missing row is not an error: a clip catalogued before the pipeline existed has none, and there
// is nothing to take off a belt it was never on.
func (s *sqlStore) MarkPipelineComplete(ctx context.Context, hash string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE filler_clip_pipeline SET disposition = ?, updated_at = ? WHERE clip_hash = ?`),
		string(filler.DispositionComplete), epoch(at), hash)
	if err != nil {
		return fmt.Errorf("mark pipeline complete %s: %w", hash, err)
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

// PipelineOverview groups storage facts, then lets the domain classify each group. SQL knows how
// to aggregate; it does not gain a second copy of the lifecycle state machine. Grouping on whether
// next_run is in the future keeps the result bounded regardless of catalog size.
func (s *sqlStore) PipelineOverview(ctx context.Context, at time.Time) (filler.PipelineOverview, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT disposition, status, stage, reject_reason,
		CASE WHEN next_run > ? THEN 1 ELSE 0 END AS scheduled, COUNT(*)
		FROM filler_clip_pipeline
		GROUP BY 1, 2, 3, 4, 5`), epoch(at))
	if err != nil {
		return filler.PipelineOverview{}, fmt.Errorf("pipeline overview: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out filler.PipelineOverview
	for rows.Next() {
		var disposition, status, stage, rejectReason string
		var scheduled, count int
		if err := rows.Scan(&disposition, &status, &stage, &rejectReason, &scheduled, &count); err != nil {
			return filler.PipelineOverview{}, fmt.Errorf("scan pipeline overview: %w", err)
		}
		row := filler.ClipPipeline{
			Disposition:  filler.Disposition(disposition),
			Status:       filler.StageStatus(status),
			Stage:        filler.StageID(stage),
			RejectReason: filler.RejectReason(rejectReason),
		}
		if scheduled != 0 {
			row.NextRun = at.Add(time.Second)
		}
		out.AddLifecycle(row.Lifecycle(at), count)
	}
	if err := rows.Err(); err != nil {
		return filler.PipelineOverview{}, fmt.Errorf("pipeline overview rows: %w", err)
	}
	return out, nil
}

// ListClipPipelines reads pipeline rows for the Incoming read model.
func (s *sqlStore) ListClipPipelines(ctx context.Context, f filler.PipelineFilter) ([]filler.ClipPipeline, error) {
	where, args, err := clipPipelineWhere(f, true)
	if err != nil {
		return nil, err
	}
	q := clipPipelineSelect + where
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
	where, args, err := clipPipelineWhere(f, false)
	if err != nil {
		return 0, err
	}
	q := `SELECT COUNT(*) FROM filler_clip_pipeline` + where
	var n int
	if err := s.db.QueryRowContext(ctx, s.ph(q), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count clip pipelines: %w", err)
	}
	return n, nil
}

// ListPreparationWork keeps the pipeline read and its coarse duration join bounded without an
// N+1 catalog lookup. Order remains ListClipPipelines' stable newest-first order.
func (s *sqlStore) ListPreparationWork(ctx context.Context, f filler.PipelineFilter) ([]filler.PreparationWork, error) {
	pipelines, err := s.ListClipPipelines(ctx, f)
	if err != nil || len(pipelines) == 0 {
		return nil, err
	}
	placeholders := make([]string, len(pipelines))
	args := make([]any, len(pipelines))
	for i, pipeline := range pipelines {
		placeholders[i], args[i] = "?", pipeline.ClipHash
	}
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT hash, duration_ms FROM clips WHERE hash IN (`+strings.Join(placeholders, `, `)+`)`), args...)
	if err != nil {
		return nil, fmt.Errorf("list preparation durations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	durations := make(map[string]int64, len(pipelines))
	for rows.Next() {
		var hash string
		var duration int64
		if err := rows.Scan(&hash, &duration); err != nil {
			return nil, fmt.Errorf("scan preparation duration: %w", err)
		}
		durations[hash] = duration
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]filler.PreparationWork, 0, len(pipelines))
	for _, pipeline := range pipelines {
		duration, ok := durations[pipeline.ClipHash]
		if ok {
			out = append(out, filler.PreparationWork{Pipeline: pipeline, DurationMs: duration})
		}
	}
	return out, nil
}

func clipPipelineWhere(f filler.PipelineFilter, includeCursor bool) (string, []any, error) {
	var where []string
	var args []any
	if len(f.Dispositions) > 0 {
		placeholders := make([]string, 0, len(f.Dispositions))
		for _, disposition := range f.Dispositions {
			placeholders = append(placeholders, `?`)
			args = append(args, string(disposition))
		}
		where = append(where, `disposition IN (`+strings.Join(placeholders, `, `)+`)`)
	}
	if !f.UpdatedAtOrAfter.IsZero() {
		where = append(where, `updated_at >= ?`)
		args = append(args, epoch(f.UpdatedAtOrAfter))
	}
	if includeCursor && (!f.BeforeUpdatedAt.IsZero() || f.BeforeClipHash != "") {
		if f.BeforeUpdatedAt.IsZero() || f.BeforeClipHash == "" {
			return "", nil, fmt.Errorf("pipeline cursor requires updated time and clip hash")
		}
		at := epoch(f.BeforeUpdatedAt)
		where = append(where, `(updated_at < ? OR (updated_at = ? AND clip_hash > ?))`)
		args = append(args, at, at, f.BeforeClipHash)
	}
	if len(where) > 0 {
		return ` WHERE ` + strings.Join(where, ` AND `), args, nil
	}
	return "", args, nil
}

// ListClipsWithoutPipeline returns catalogued clips with no pipeline row yet, in hash order.
//
// It picks the batch's hashes here, where the pipeline table is, and reads the clips themselves
// through the core's ListClips, which owns how a clip row is read. `NOT EXISTS` rather than a join
// keeps the pick to one column. The batch is bounded by limit, so the Hashes read stays far below
// Postgres's 65535-parameter cap.
//
// ⚠ Removed clips are excluded. A tombstoned clip is not work: enrolling it would run the whole
// ladder against something deliberately taken out of rotation. Held clips and composites are
// work, so the read includes them.
func (s *sqlStore) ListClipsWithoutPipeline(ctx context.Context, limit int) ([]filler.StoreClip, error) {
	q := `SELECT hash FROM clips WHERE removed_at = 0 AND NOT EXISTS (
			SELECT 1 FROM filler_clip_pipeline p WHERE p.clip_hash = clips.hash)
		ORDER BY hash`
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
	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("scan clip without pipeline: %w", err)
		}
		hashes = append(hashes, hash)
	}
	if err := rows.Err(); err != nil || len(hashes) == 0 {
		return nil, err
	}
	clips, err := s.core.ListClips(ctx, store.ClipFilter{Hashes: hashes, IncludeHeld: true, IncludeComposites: true})
	if err != nil {
		return nil, fmt.Errorf("list clips without pipeline: %w", err)
	}
	sort.Slice(clips, func(i, j int) bool { return clips[i].Hash < clips[j].Hash })
	out := make([]filler.StoreClip, 0, len(clips))
	for _, c := range clips {
		out = append(out, filler.StoreClip{Clip: c.Clip, UpdatedAt: c.UpdatedAt})
	}
	return out, nil
}

// pruneOrphanPipelines deletes pipeline rows whose clip is gone.
//
// ⚠ `filler_clip_pipeline` is a sibling of `clips` with no foreign key — deliberately, so it
// survives a `clips` rebuild — and the price of that independence is that nothing else will ever
// clean it up. An orphan row is not inert either: `ListPipelineWork` would keep returning it,
// `advance` would fail to find the clip, and it would be re-tombstoned as "no longer in the
// catalog" on every pass, forever.
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
