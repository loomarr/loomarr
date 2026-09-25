package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

const diagnosticEventColumns = `id, occurred_at, received_at, level, source, subsystem, event,
message, request_id, playback_session_id, channel_id, schedule_block_id, job_id, process_run_id,
actor_id, instance_id, attributes_json, size_bytes`

// AppendDiagnosticEvents persists one recorder batch atomically. A partial batch would make the
// drop/failure accounting lie, so one failed row rolls back the whole append and the recorder's
// stdout-only failure hook reports the batch once.
func (s *sqlStore) AppendDiagnosticEvents(ctx context.Context, records []diagnostics.Record) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin diagnostic event batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	query := s.ph(`INSERT INTO diagnostic_events (` + diagnosticEventColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	var added int64
	for _, record := range records {
		if record.ID == "" || record.Event == "" {
			return fmt.Errorf("append diagnostic events: id and event are required")
		}
		if _, err := tx.ExecContext(ctx, query,
			record.ID, record.OccurredAt, record.ReceivedAt, record.Level, record.Source,
			record.Subsystem, record.Event, record.Message, record.RequestID,
			record.PlaybackSessionID, record.ChannelID, record.ScheduleBlockID, record.JobID,
			record.ProcessRunID, record.ActorID, record.InstanceID, record.AttributesJSON,
			record.SizeBytes,
		); err != nil {
			return fmt.Errorf("append diagnostic event %s: %w", record.ID, err)
		}
		added += record.SizeBytes
	}
	if err := s.addRetainedBytes(ctx, tx, retainedEvents, added); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit diagnostic event batch: %w", err)
	}
	return nil
}

// ListDiagnosticEvents returns newest-first retained records for conformance and the initial
// bounded read seam. #511 extends this with the typed filtered cursor query used by the UI/agents.
func (s *sqlStore) ListDiagnosticEvents(ctx context.Context, limit int) ([]diagnostics.Record, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT `+diagnosticEventColumns+`
		FROM diagnostic_events ORDER BY occurred_at DESC, id DESC LIMIT ?`), limit)
	if err != nil {
		return nil, fmt.Errorf("list diagnostic events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]diagnostics.Record, 0, limit)
	for rows.Next() {
		record, err := scanDiagnosticEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

// QueryDiagnosticEvents applies one fully validated, bounded query from the diagnostics module.
// The time predicate is mandatory even when indexed identity filters are present, which prevents a
// caller-controlled combination from degrading into an unbounded retained-history scan.
func (s *sqlStore) QueryDiagnosticEvents(
	ctx context.Context, query diagnostics.EventStoreQuery,
) ([]diagnostics.Record, error) {
	if query.From < 0 || query.To <= query.From || query.Limit < 1 || query.Limit > 201 {
		return nil, fmt.Errorf("query diagnostic events: invalid module query")
	}
	order := query.Order
	if order == "" {
		order = diagnostics.EventOrderNewest
	}
	if order != diagnostics.EventOrderNewest && order != diagnostics.EventOrderOldest {
		return nil, fmt.Errorf("query diagnostic events: invalid module query")
	}
	text, args := diagnosticEventQuery(query)
	rows, err := s.db.QueryContext(ctx, s.ph(text), args...)
	if err != nil {
		return nil, fmt.Errorf("query diagnostic events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]diagnostics.Record, 0, query.Limit)
	for rows.Next() {
		record, err := scanDiagnosticEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query diagnostic events rows: %w", err)
	}
	return out, nil
}

// diagnosticEventQuery builds the validated filter query, split out so a test can EXPLAIN exactly
// what production runs. The query is already validated by QueryDiagnosticEvents.
func diagnosticEventQuery(query diagnostics.EventStoreQuery) (string, []any) {
	comparison, direction := "<", "DESC"
	if query.Order == diagnostics.EventOrderOldest {
		comparison, direction = ">", "ASC"
	}
	clauses := []string{"occurred_at >= ?", "occurred_at <= ?"}
	args := []any{query.From, query.To}
	addExact := func(column, value string) {
		if value == "" {
			return
		}
		clauses = append(clauses, column+" = ?")
		args = append(args, value)
	}
	// The correlation-id indexes are partial (`WHERE col <> ''`, migration 00121). The planner
	// cannot prove `col = ?` implies `col <> ''` for a bound parameter, so the redundant term is
	// what lets it pick them.
	addCorrelation := func(column, value string) {
		if value == "" {
			return
		}
		clauses = append(clauses, column+" <> ''")
		addExact(column, value)
	}
	addExact("level", string(query.Level))
	addExact("source", string(query.Source))
	addExact("event", query.Event)
	addExact("subsystem", query.Subsystem)
	addCorrelation("request_id", query.RequestID)
	addCorrelation("playback_session_id", query.PlaybackSessionID)
	addCorrelation("channel_id", query.ChannelID)
	addCorrelation("schedule_block_id", query.ScheduleBlockID)
	addCorrelation("job_id", query.JobID)
	addCorrelation("process_run_id", query.ProcessRunID)
	addExact("instance_id", query.InstanceID)
	if query.CursorID != "" {
		clauses = append(clauses, "(occurred_at "+comparison+" ? OR (occurred_at = ? AND id "+comparison+" ?))")
		args = append(args, query.CursorOccurredAt, query.CursorOccurredAt, query.CursorID)
	}
	if query.Text != "" {
		literal := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(query.Text))
		pattern := "%" + literal + "%"
		clauses = append(clauses, `(LOWER(event) LIKE ? ESCAPE '\' OR LOWER(message) LIKE ? ESCAPE '\' OR
			LOWER(subsystem) LIKE ? ESCAPE '\' OR LOWER(attributes_json) LIKE ? ESCAPE '\')`)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	args = append(args, query.Limit)
	return `SELECT ` + diagnosticEventColumns + `
		FROM diagnostic_events WHERE ` + strings.Join(clauses, " AND ") + `
		ORDER BY occurred_at ` + direction + `, id ` + direction + ` LIMIT ?`, args
}

func scanDiagnosticEvent(row interface{ Scan(...any) error }) (diagnostics.Record, error) {
	var record diagnostics.Record
	if err := row.Scan(
		&record.ID, &record.OccurredAt, &record.ReceivedAt, &record.Level, &record.Source,
		&record.Subsystem, &record.Event, &record.Message, &record.RequestID,
		&record.PlaybackSessionID, &record.ChannelID, &record.ScheduleBlockID, &record.JobID,
		&record.ProcessRunID, &record.ActorID, &record.InstanceID, &record.AttributesJSON,
		&record.SizeBytes,
	); err != nil {
		return diagnostics.Record{}, fmt.Errorf("scan diagnostic event: %w", err)
	}
	return record, nil
}

const diagnosticProcessColumns = `id, purpose, parent_run_id, instance_id, channel_id, target,
schedule_block_id, job_id, executable, executable_version, command_summary, started_at, ended_at,
status, exit_code, termination_reason, first_error, last_error, output_ref, output_bytes,
discarded_lines, updated_at, size_bytes`

// UpsertDiagnosticProcessRun inserts a run at start and replaces its bounded lifecycle snapshot as
// progress/termination arrives. The run id is the identity; status transitions are enforced by the
// diagnostics module, while both SQL adapters share this one persistence path.
func (s *sqlStore) UpsertDiagnosticProcessRun(ctx context.Context, run diagnostics.ProcessRun) error {
	if run.ID == "" || run.Purpose == "" || run.StartedAt == 0 {
		return fmt.Errorf("upsert diagnostic process run: id, purpose, and started_at are required")
	}
	switch run.Status {
	case diagnostics.ProcessRunning, diagnostics.ProcessSucceeded, diagnostics.ProcessFailed,
		diagnostics.ProcessCancelled, diagnostics.ProcessInterrupted:
	default:
		return fmt.Errorf("upsert diagnostic process run %s: invalid status %q", run.ID, run.Status)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert diagnostic process run %s: %w", run.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	// The running total moves by the row's size CHANGE, so read the old size in the same tx.
	var previous int64
	switch err := tx.QueryRowContext(ctx, s.ph(`SELECT size_bytes FROM diagnostic_process_runs WHERE id = ?`), run.ID).Scan(&previous); {
	case err == sql.ErrNoRows:
	case err != nil:
		return fmt.Errorf("upsert diagnostic process run %s: %w", run.ID, err)
	}
	_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO diagnostic_process_runs (`+diagnosticProcessColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			purpose = excluded.purpose, parent_run_id = excluded.parent_run_id,
			instance_id = excluded.instance_id, channel_id = excluded.channel_id,
			target = excluded.target, schedule_block_id = excluded.schedule_block_id,
			job_id = excluded.job_id, executable = excluded.executable,
			executable_version = excluded.executable_version,
			command_summary = excluded.command_summary, started_at = excluded.started_at,
			ended_at = excluded.ended_at, status = excluded.status, exit_code = excluded.exit_code,
			termination_reason = excluded.termination_reason, first_error = excluded.first_error,
			last_error = excluded.last_error, output_ref = excluded.output_ref,
			output_bytes = excluded.output_bytes, discarded_lines = excluded.discarded_lines,
			updated_at = excluded.updated_at, size_bytes = excluded.size_bytes`),
		run.ID, run.Purpose, run.ParentRunID, run.InstanceID, run.ChannelID, run.Target,
		run.ScheduleBlockID, run.JobID, run.Executable, run.ExecutableVersion, run.CommandSummary,
		run.StartedAt, run.EndedAt, run.Status, diagnosticNullableInt(run.ExitCode), run.TerminationReason,
		run.FirstError, run.LastError, run.OutputRef, run.OutputBytes, run.DiscardedLines,
		run.UpdatedAt, run.SizeBytes,
	)
	if err != nil {
		return fmt.Errorf("upsert diagnostic process run %s: %w", run.ID, err)
	}
	if err := s.addRetainedBytes(ctx, tx, retainedProcessRuns, run.SizeBytes-previous); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert diagnostic process run %s: %w", run.ID, err)
	}
	return nil
}

// GetDiagnosticProcessRun returns one run or ErrNotFound.
func (s *sqlStore) GetDiagnosticProcessRun(ctx context.Context, id string) (diagnostics.ProcessRun, error) {
	row := s.db.QueryRowContext(ctx, s.ph(`SELECT `+diagnosticProcessColumns+`
		FROM diagnostic_process_runs WHERE id = ?`), id)
	run, err := scanDiagnosticProcessRun(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return diagnostics.ProcessRun{}, ErrNotFound
		}
		return diagnostics.ProcessRun{}, fmt.Errorf("get diagnostic process run %s: %w", id, err)
	}
	return run, nil
}

// FindDiagnosticProcessRun is the diagnostics module's lookup seam. It keeps store.ErrNotFound out
// of the domain package while preserving GetDiagnosticProcessRun for store callers and conformance.
func (s *sqlStore) FindDiagnosticProcessRun(ctx context.Context, id string) (diagnostics.ProcessRun, bool, error) {
	run, err := s.GetDiagnosticProcessRun(ctx, id)
	if err == ErrNotFound {
		return diagnostics.ProcessRun{}, false, nil
	}
	if err != nil {
		return diagnostics.ProcessRun{}, false, err
	}
	return run, true, nil
}

// QueryDiagnosticProcessRuns applies the diagnostics module's mandatory bounded time window and
// exact filters. The sentinel limit and opaque cursor are resolved above this adapter.
func (s *sqlStore) QueryDiagnosticProcessRuns(
	ctx context.Context, query diagnostics.ProcessStoreQuery,
) ([]diagnostics.ProcessRun, error) {
	if query.From < 0 || query.To <= query.From || query.Limit < 1 || query.Limit > 201 {
		return nil, fmt.Errorf("query diagnostic process runs: invalid module query")
	}
	clauses := []string{"started_at >= ?", "started_at <= ?"}
	args := []any{query.From, query.To}
	addExact := func(column, value string) {
		if value == "" {
			return
		}
		clauses = append(clauses, column+" = ?")
		args = append(args, value)
	}
	addExact("status", string(query.Status))
	addExact("purpose", query.Purpose)
	addExact("channel_id", query.ChannelID)
	addExact("job_id", query.JobID)
	if query.CursorID != "" {
		clauses = append(clauses, "(started_at < ? OR (started_at = ? AND id < ?))")
		args = append(args, query.CursorStartedAt, query.CursorStartedAt, query.CursorID)
	}
	args = append(args, query.Limit)
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT `+diagnosticProcessColumns+`
		FROM diagnostic_process_runs WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY started_at DESC, id DESC LIMIT ?`), args...)
	if err != nil {
		return nil, fmt.Errorf("query diagnostic process runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	runs := make([]diagnostics.ProcessRun, 0, query.Limit)
	for rows.Next() {
		run, err := scanDiagnosticProcessRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan diagnostic process run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query diagnostic process run rows: %w", err)
	}
	return runs, nil
}

func scanDiagnosticProcessRun(row interface{ Scan(...any) error }) (diagnostics.ProcessRun, error) {
	var run diagnostics.ProcessRun
	var exitCode sql.NullInt64
	if err := row.Scan(
		&run.ID, &run.Purpose, &run.ParentRunID, &run.InstanceID, &run.ChannelID, &run.Target,
		&run.ScheduleBlockID, &run.JobID, &run.Executable, &run.ExecutableVersion,
		&run.CommandSummary, &run.StartedAt, &run.EndedAt, &run.Status, &exitCode,
		&run.TerminationReason, &run.FirstError, &run.LastError, &run.OutputRef,
		&run.OutputBytes, &run.DiscardedLines, &run.UpdatedAt, &run.SizeBytes,
	); err != nil {
		return diagnostics.ProcessRun{}, err
	}
	if exitCode.Valid {
		value := int(exitCode.Int64)
		run.ExitCode = &value
	}
	return run, nil
}

func diagnosticNullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

// diagnosticRetentionCandidatesQuery is the page query and its arguments, split out so a test can
// EXPLAIN exactly what production runs.
//
// ⚠ Each branch is ORDERED AND LIMITED INSIDE its own subquery. The first cut ordered the UNION
// outside, which made SQLite scan and sort every diagnostic_events row to return one 256-row
// page — on the 2.7M-row household table that is seconds per page holding the only connection,
// repeated for every page, which is why the nightly purge never finished (#1411). Bounded per
// branch, each side walks its own index (idx_diagnostic_events_time) and stops after `limit`
// rows; the outer merge sorts at most 2×limit. A zero beforeMS ("everything deletable") is
// resolved here in Go, not as an `OR ? = 0` in SQL, because that OR defeats the index range.
func diagnosticRetentionCandidatesQuery(beforeMS int64, limit int) (string, []any) {
	eventWhere, runWhere := "", "WHERE status <> 'running' AND ended_at > 0"
	var eventArgs, runArgs []any
	if beforeMS != 0 {
		eventWhere, runWhere = "WHERE occurred_at < ?", runWhere+" AND ended_at < ?"
		eventArgs, runArgs = []any{beforeMS}, []any{beforeMS}
	}
	args := append(append(append(eventArgs, limit), append(runArgs, limit)...), limit)
	return `SELECT kind, id, at, size_bytes, output_ref FROM (
		SELECT kind, id, at, size_bytes, output_ref FROM (
			SELECT 'event' AS kind, id, occurred_at AS at, size_bytes, '' AS output_ref
			FROM diagnostic_events ` + eventWhere + ` ORDER BY occurred_at, id LIMIT ?
		) e
		UNION ALL
		SELECT kind, id, at, size_bytes, output_ref FROM (
			SELECT 'process_run' AS kind, id, ended_at AS at, size_bytes, output_ref
			FROM diagnostic_process_runs ` + runWhere + ` ORDER BY ended_at, id LIMIT ?
		) r
	) candidates ORDER BY at, id LIMIT ?`, args
}

// DeleteDiagnosticEvents removes one bounded batch in a single statement and reports how many
// rows it deleted. Callers page (ListDiagnosticRetentionCandidates caps a page at 1000) and
// yield between batches; deleting row by row cost a round trip and a full 11-index update each.
func (s *sqlStore) DeleteDiagnosticEvents(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if len(ids) > diagnosticDeleteBatchMax {
		return 0, fmt.Errorf("delete diagnostic events: batch of %d exceeds %d", len(ids), diagnosticDeleteBatchMax)
	}
	n, err := s.deleteByID(ctx, retainedEvents, ids)
	if err != nil {
		return 0, fmt.Errorf("delete %d diagnostic events: %w", len(ids), err)
	}
	return n, nil
}

// diagnosticDeleteBatchMax bounds one delete transaction. 500 rows × 11 indexes is tens of
// milliseconds on the household disk, so any other waiter for the connection is served promptly.
const diagnosticDeleteBatchMax = 1000

// ListDiagnosticRetentionCandidates returns one oldest-first page. A zero before selects all
// deletable evidence for the storage-budget phase; active Process runs are never candidates.
func (s *sqlStore) ListDiagnosticRetentionCandidates(
	ctx context.Context, before time.Time, limit int,
) ([]diagnostics.RetentionCandidate, error) {
	if limit <= 0 || limit > 1000 {
		limit = 256
	}
	beforeMS := before.UnixMilli()
	if before.IsZero() {
		beforeMS = 0
	}
	query, args := diagnosticRetentionCandidatesQuery(beforeMS, limit)
	rows, err := s.db.QueryContext(ctx, s.ph(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list diagnostic retention candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]diagnostics.RetentionCandidate, 0, limit)
	for rows.Next() {
		var candidate diagnostics.RetentionCandidate
		if err := rows.Scan(&candidate.Kind, &candidate.ID, &candidate.At, &candidate.SizeBytes, &candidate.OutputRef); err != nil {
			return nil, fmt.Errorf("scan diagnostic retention candidate: %w", err)
		}
		result = append(result, candidate)
	}
	return result, rows.Err()
}

// FinalizeStaleDiagnosticProcessRuns marks every run this instance left "running" as interrupted.
// Called once at startup, before the new process records any run: an instance id is stable across
// restarts, so a run still "running" then belongs to a process that died without finalizing it.
// Left alone it is never purged (retention skips running rows).
//
// ASSUMPTION: Loomarr supports exactly one process per database (see
// docs/engineering/plans/multi-replica-readiness.md), so matching on instance_id alone cannot
// finalize a live sibling's runs. Multi-replica support must scope this by process liveness.
func (s *sqlStore) FinalizeStaleDiagnosticProcessRuns(ctx context.Context, instanceID string, now time.Time) (int, error) {
	ms := now.UnixMilli()
	result, err := s.db.ExecContext(ctx, s.ph(`UPDATE diagnostic_process_runs
		SET status = ?, ended_at = ?, updated_at = ?, termination_reason = ?
		WHERE status = ? AND instance_id = ?`),
		diagnostics.ProcessInterrupted, ms, ms, "process exited without finalizing this run",
		diagnostics.ProcessRunning, instanceID)
	if err != nil {
		return 0, fmt.Errorf("finalize stale diagnostic process runs: %w", err)
	}
	return rowsAffected(result), nil
}

// DeleteDiagnosticProcessRun repeats the terminal-state guard at the destructive boundary.
func (s *sqlStore) DeleteDiagnosticProcessRun(ctx context.Context, id string) (bool, error) {
	n, err := s.deleteCounted(ctx, retainedProcessRuns,
		`DELETE FROM diagnostic_process_runs WHERE id = ? AND status <> 'running'`, id)
	if err != nil {
		return false, fmt.Errorf("delete diagnostic process run %s: %w", id, err)
	}
	return n > 0, nil
}

// retainedScope names one row of diagnostic_retained_bytes: the running total of size_bytes for one
// evidence table. Every write to diagnostic_events / diagnostic_process_runs moves its total in the
// SAME transaction (addRetainedBytes / deleteCounted), so the two rows are always the table SUMs
// without anyone scanning the tables (#1398: a SUM over 2.7M rows per housekeeping run).
type retainedScope struct{ scope, table string }

var (
	retainedEvents      = retainedScope{"events", "diagnostic_events"}
	retainedProcessRuns = retainedScope{"process_runs", "diagnostic_process_runs"}
)

const diagnosticRetainedBytesQuery = `SELECT COALESCE(SUM(bytes), 0) FROM diagnostic_retained_bytes`

// addRetainedBytes applies a signed delta to one scope's total inside the caller's transaction.
func (s *sqlStore) addRetainedBytes(ctx context.Context, tx *sql.Tx, scope retainedScope, delta int64) error {
	if delta == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE diagnostic_retained_bytes SET bytes = bytes + ? WHERE scope = ?`),
		delta, scope.scope); err != nil {
		return fmt.Errorf("update retained diagnostics total (%s): %w", scope.scope, err)
	}
	return nil
}

// deleteCounted runs a DELETE on scope's table, learns the freed bytes from RETURNING, and moves
// the running total by them in one transaction. The query must delete only from scope.table.
func (s *sqlStore) deleteCounted(ctx context.Context, scope retainedScope, query string, args ...any) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, s.ph(query+` RETURNING size_bytes`), args...)
	if err != nil {
		return 0, err
	}
	var n int
	var freed int64
	for rows.Next() {
		var size int64
		if err := rows.Scan(&size); err != nil {
			_ = rows.Close()
			return 0, err
		}
		n++
		freed += size
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := s.addRetainedBytes(ctx, tx, scope, -freed); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

func (s *sqlStore) DiagnosticRetainedBytes(ctx context.Context) (int64, error) {
	var retained int64
	err := s.db.QueryRowContext(ctx, diagnosticRetainedBytesQuery).Scan(&retained)
	if err != nil {
		return 0, fmt.Errorf("measure retained diagnostics: %w", err)
	}
	return retained, nil
}

// diagnosticPurgeBatch is the row count of one PurgeDiagnostics delete statement.
const diagnosticPurgeBatch = 500

// PurgeDiagnostics applies the SQL-owned half of §5 retention. File-backed completed runs are
// deliberately excluded: #512's diagnostics-owned cleaner removes their opaque output first, then
// their row. Deleting the row here would orphan a file the store cannot resolve safely.
//
// ⚠ **It works in bounded batches, one statement each, and never holds a transaction across
// batches.** It used to run as ONE transaction; on SQLite (MaxOpenConns 1) that transaction owned
// the only connection for the entire delete, so River's leader election and the health probe
// queued behind it and timed out (#1411). Between batches the connection returns to the pool, and
// database/sql hands it to the longest waiter before this loop's next Exec, so other work is
// delayed by one batch (tens of ms), not by the whole purge. The price is that a mid-purge
// failure leaves a prefix deleted — harmless, since retention is idempotent and every batch is
// oldest-first.
func (s *sqlStore) PurgeDiagnostics(ctx context.Context, before time.Time, maxBytes int64) (diagnostics.PurgeResult, error) {
	result := diagnostics.PurgeResult{}
	beforeMS := before.UnixMilli()
	for {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("purge expired diagnostic events: %w", err)
		}
		n, err := s.deleteCounted(ctx, retainedEvents, `DELETE FROM diagnostic_events WHERE id IN (
			SELECT id FROM diagnostic_events WHERE occurred_at < ? ORDER BY occurred_at, id LIMIT ?)`,
			beforeMS, diagnosticPurgeBatch)
		if err != nil {
			return result, fmt.Errorf("purge expired diagnostic events: %w", err)
		}
		result.Events += n
		if n < diagnosticPurgeBatch {
			break
		}
	}
	var err error
	result.ProcessRuns, err = s.deleteCounted(ctx, retainedProcessRuns, `DELETE FROM diagnostic_process_runs
		WHERE status <> 'running' AND ended_at > 0 AND ended_at < ? AND output_ref = ''`, beforeMS)
	if err != nil {
		return result, fmt.Errorf("purge expired diagnostic process runs: %w", err)
	}

	retained, err := s.DiagnosticRetainedBytes(ctx)
	if err != nil {
		return result, err
	}
	// `retained` is decremented locally as the loop deletes; the stored total is decremented by the
	// same rows inside each delete, so the two stay equal.
	for maxBytes > 0 && retained > maxBytes {
		page, err := s.diagnosticBudgetPage(ctx, diagnosticPurgeBatch)
		if err != nil {
			return result, err
		}
		if len(page) == 0 {
			break // only active or file-backed evidence remains; it may hold the install above budget.
		}
		var eventIDs, runIDs []string
		for _, item := range page {
			if retained <= maxBytes {
				break
			}
			if item.kind == "process" {
				runIDs = append(runIDs, item.id)
			} else {
				eventIDs = append(eventIDs, item.id)
			}
			retained -= item.size
		}
		events, err := s.deleteByID(ctx, retainedEvents, eventIDs)
		if err != nil {
			return result, fmt.Errorf("purge diagnostic budget candidates: %w", err)
		}
		runs, err := s.deleteByID(ctx, retainedProcessRuns, runIDs)
		if err != nil {
			return result, fmt.Errorf("purge diagnostic budget candidates: %w", err)
		}
		result.Events += events
		result.ProcessRuns += runs
	}
	result.RetainedBytes = max(0, retained)
	return result, nil
}

type diagnosticBudgetItem struct {
	kind, id string
	size     int64
}

// diagnosticBudgetPage returns the oldest deletable rows, each branch bounded by its own index
// walk for the reason diagnosticRetentionCandidatesQuery documents.
func (s *sqlStore) diagnosticBudgetPage(ctx context.Context, limit int) ([]diagnosticBudgetItem, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT kind, id, size_bytes FROM (
		SELECT kind, id, at, size_bytes FROM (
			SELECT 'event' AS kind, id, occurred_at AS at, size_bytes
			FROM diagnostic_events ORDER BY occurred_at, id LIMIT ?
		) e
		UNION ALL
		SELECT kind, id, at, size_bytes FROM (
			SELECT 'process' AS kind, id, started_at AS at, size_bytes
			FROM diagnostic_process_runs WHERE status <> 'running' AND output_ref = ''
			ORDER BY started_at, id LIMIT ?
		) r
	) retained ORDER BY at, id LIMIT ?`), limit, limit, limit)
	if err != nil {
		return nil, fmt.Errorf("list diagnostic budget candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var page []diagnosticBudgetItem
	for rows.Next() {
		var item diagnosticBudgetItem
		if err := rows.Scan(&item.kind, &item.id, &item.size); err != nil {
			return nil, fmt.Errorf("scan diagnostic budget candidate: %w", err)
		}
		page = append(page, item)
	}
	return page, rows.Err()
}

// deleteByID removes the named rows of one evidence table in a single counted statement. The table
// comes from a retainedScope literal in this file, never caller input.
func (s *sqlStore) deleteByID(ctx context.Context, scope retainedScope, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return s.deleteCounted(ctx, scope, `DELETE FROM `+scope.table+` WHERE id IN (?`+
		strings.Repeat(", ?", len(ids)-1)+`)`, args...)
}
