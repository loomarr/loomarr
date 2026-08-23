package store

import (
	"context"
	"database/sql"
	"fmt"
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
