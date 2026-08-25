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
	comparison, direction := "<", "DESC"
	if order == diagnostics.EventOrderOldest {
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
	addExact("level", string(query.Level))
	addExact("source", string(query.Source))
	addExact("event", query.Event)
	addExact("subsystem", query.Subsystem)
	addExact("request_id", query.RequestID)
	addExact("playback_session_id", query.PlaybackSessionID)
	addExact("channel_id", query.ChannelID)
	addExact("schedule_block_id", query.ScheduleBlockID)
	addExact("job_id", query.JobID)
	addExact("process_run_id", query.ProcessRunID)
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
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT `+diagnosticEventColumns+`
		FROM diagnostic_events WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY occurred_at `+direction+`, id `+direction+` LIMIT ?`), args...)
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
	case diagnostics.ProcessRunning, diagnostics.ProcessSucceeded, diagnostics.ProcessFailed, diagnostics.ProcessCancelled:
	default:
		return fmt.Errorf("upsert diagnostic process run %s: invalid status %q", run.ID, run.Status)
	}
	_, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO diagnostic_process_runs (`+diagnosticProcessColumns+`)
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
	rows, err := s.db.QueryContext(ctx, s.ph(`SELECT kind, id, at, size_bytes, output_ref FROM (
		SELECT 'event' AS kind, id, occurred_at AS at, size_bytes, '' AS output_ref
		FROM diagnostic_events WHERE (CAST(? AS BIGINT) = 0 OR occurred_at < CAST(? AS BIGINT))
		UNION ALL
		SELECT 'process_run' AS kind, id, ended_at AS at, size_bytes, output_ref
		FROM diagnostic_process_runs
		WHERE status <> 'running' AND ended_at > 0
			AND (CAST(? AS BIGINT) = 0 OR ended_at < CAST(? AS BIGINT))
	) candidates ORDER BY at, id LIMIT ?`), beforeMS, beforeMS, beforeMS, beforeMS, limit)
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

func (s *sqlStore) DeleteDiagnosticEvent(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM diagnostic_events WHERE id = ?`), id)
	if err != nil {
		return false, fmt.Errorf("delete diagnostic event %s: %w", id, err)
	}
	return rowsAffected(result) > 0, nil
}

// DeleteDiagnosticProcessRun repeats the terminal-state guard at the destructive boundary.
func (s *sqlStore) DeleteDiagnosticProcessRun(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM diagnostic_process_runs WHERE id = ? AND status <> 'running'`), id)
	if err != nil {
		return false, fmt.Errorf("delete diagnostic process run %s: %w", id, err)
	}
	return rowsAffected(result) > 0, nil
}

func (s *sqlStore) DiagnosticRetainedBytes(ctx context.Context) (int64, error) {
	var retained int64
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE((SELECT SUM(size_bytes) FROM diagnostic_events), 0) +
		COALESCE((SELECT SUM(size_bytes) FROM diagnostic_process_runs), 0)`).Scan(&retained)
	if err != nil {
		return 0, fmt.Errorf("measure retained diagnostics: %w", err)
	}
	return retained, nil
}

// PurgeDiagnostics applies the SQL-owned half of §5 retention. File-backed completed runs are
// deliberately excluded: #512's diagnostics-owned cleaner removes their opaque output first, then
// their row. Deleting the row here would orphan a file the store cannot resolve safely.
func (s *sqlStore) PurgeDiagnostics(ctx context.Context, before time.Time, maxBytes int64) (diagnostics.PurgeResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return diagnostics.PurgeResult{}, fmt.Errorf("begin diagnostics purge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result := diagnostics.PurgeResult{}
	eventResult, err := tx.ExecContext(ctx, s.ph(`DELETE FROM diagnostic_events WHERE occurred_at < ?`), before.UnixMilli())
	if err != nil {
		return result, fmt.Errorf("purge expired diagnostic events: %w", err)
	}
	result.Events = rowsAffected(eventResult)
	processResult, err := tx.ExecContext(ctx, s.ph(`DELETE FROM diagnostic_process_runs
		WHERE status <> 'running' AND ended_at > 0 AND ended_at < ? AND output_ref = ''`), before.UnixMilli())
	if err != nil {
		return result, fmt.Errorf("purge expired diagnostic process runs: %w", err)
	}
	result.ProcessRuns = rowsAffected(processResult)

	var retained int64
	if err := tx.QueryRowContext(ctx, `SELECT
		COALESCE((SELECT SUM(size_bytes) FROM diagnostic_events), 0) +
		COALESCE((SELECT SUM(size_bytes) FROM diagnostic_process_runs), 0)`).Scan(&retained); err != nil {
		return result, fmt.Errorf("measure retained diagnostics: %w", err)
	}
	if maxBytes > 0 && retained > maxBytes {
		rows, err := tx.QueryContext(ctx, `SELECT kind, id, size_bytes FROM (
			SELECT 'event' AS kind, id, occurred_at AS at, size_bytes FROM diagnostic_events
			UNION ALL
			SELECT 'process' AS kind, id, started_at AS at, size_bytes FROM diagnostic_process_runs
			WHERE status <> 'running' AND output_ref = ''
		) retained ORDER BY at, id`)
		if err != nil {
			return result, fmt.Errorf("list diagnostic budget candidates: %w", err)
		}
		type candidate struct {
			kind, id string
			size     int64
		}
		var candidates []candidate
		for rows.Next() {
			var item candidate
			if err := rows.Scan(&item.kind, &item.id, &item.size); err != nil {
				_ = rows.Close()
				return result, fmt.Errorf("scan diagnostic budget candidate: %w", err)
			}
			candidates = append(candidates, item)
		}
		if err := rows.Close(); err != nil {
			return result, fmt.Errorf("close diagnostic budget candidates: %w", err)
		}
		for _, item := range candidates {
			if retained <= maxBytes {
				break
			}
			table := "diagnostic_events"
			if item.kind == "process" {
				table = "diagnostic_process_runs"
			}
			if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM `+table+` WHERE id = ?`), item.id); err != nil {
				return result, fmt.Errorf("purge diagnostic budget candidate %s: %w", item.id, err)
			}
			retained -= item.size
			if item.kind == "process" {
				result.ProcessRuns++
			} else {
				result.Events++
			}
		}
	}
	result.RetainedBytes = max(0, retained)
	if err := tx.Commit(); err != nil {
		return diagnostics.PurgeResult{}, fmt.Errorf("commit diagnostics purge: %w", err)
	}
	return result, nil
}

func rowsAffected(result sql.Result) int {
	count, err := result.RowsAffected()
	if err != nil {
		return 0
	}
	return int(count)
}
