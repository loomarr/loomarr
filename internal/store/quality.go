package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/quality"
)

const qualityExportSchemaVersion = 1

func (s *sqlStore) PutQualityRunSnapshot(ctx context.Context, snapshot quality.RunSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO quality_run_snapshots
		(id, schema_version, corpus_version, requested_model, resolved_model, provider,
		 budget_profile, application_version, accounting_available, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`),
		snapshot.ID, snapshot.SchemaVersion, snapshot.CorpusVersion, snapshot.RequestedModel,
		snapshot.ResolvedModel, string(snapshot.Provider), snapshot.BudgetProfile,
		snapshot.ApplicationVersion, snapshot.AccountingAvailable, snapshot.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("put quality run snapshot: %w", err)
	}
	stored, err := s.getQualityRunSnapshot(ctx, s.db, snapshot.ID)
	if err != nil {
		return fmt.Errorf("read quality run snapshot after put: %w", err)
	}
	if !sameQualityRunSnapshot(stored, snapshot) {
		return ErrQualitySnapshotConflict
	}
	return nil
}

func sameQualityRunSnapshot(a, b quality.RunSnapshot) bool {
	return a.ID == b.ID && a.SchemaVersion == b.SchemaVersion && a.CorpusVersion == b.CorpusVersion &&
		a.RequestedModel == b.RequestedModel && a.ResolvedModel == b.ResolvedModel && a.Provider == b.Provider &&
		a.BudgetProfile == b.BudgetProfile && a.ApplicationVersion == b.ApplicationVersion &&
		a.AccountingAvailable == b.AccountingAvailable && a.CreatedAt.Unix() == b.CreatedAt.Unix()
}

func (s *sqlStore) RecordQualityObservation(ctx context.Context, observation quality.Observation) error {
	if err := observation.Validate(); err != nil {
		return err
	}
	if observation.RunSnapshotID != "" {
		if _, err := s.getQualityRunSnapshot(ctx, s.db, observation.RunSnapshotID); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO quality_receipts
		(idempotency_key, occurred_at, stage, outcome, duration_millis, tool_calls,
		 candidate_count, cost_nanos, run_snapshot_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (idempotency_key) DO NOTHING`),
		observation.IdempotencyKey, observation.At.Unix(), string(observation.Stage), string(observation.Outcome),
		observation.Duration.Milliseconds(), observation.ToolCalls, observation.CandidateCount,
		observation.CostNanos, nullableQualitySnapshotID(observation.RunSnapshotID))
	if err != nil {
		return fmt.Errorf("record quality observation: %w", err)
	}
	return nil
}

type qualityRollupKey struct {
	day, stage, outcome, snapshotID string
}

func (s *sqlStore) MaintainQualityLedger(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("maintain quality ledger: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	receiptCutoff := now.UTC().Add(-30 * 24 * time.Hour).Unix()
	rows, err := tx.QueryContext(ctx, s.ph(`SELECT occurred_at, stage, outcome, run_snapshot_id,
		duration_millis, tool_calls, candidate_count, cost_nanos
		FROM quality_receipts WHERE occurred_at < ? ORDER BY occurred_at, idempotency_key`), receiptCutoff)
	if err != nil {
		return fmt.Errorf("maintain quality ledger: list receipts: %w", err)
	}
	rollups := make(map[qualityRollupKey]quality.Aggregate)
	for rows.Next() {
		var at int64
		var stage, outcome string
		var snapshotID sql.NullString
		var durationMillis, toolCalls, candidateCount, costNanos int64
		if err := rows.Scan(&at, &stage, &outcome, &snapshotID, &durationMillis, &toolCalls, &candidateCount, &costNanos); err != nil {
			_ = rows.Close()
			return fmt.Errorf("maintain quality ledger: scan receipt: %w", err)
		}
		key := qualityRollupKey{day: time.Unix(at, 0).UTC().Format(time.DateOnly), stage: stage, outcome: outcome, snapshotID: snapshotID.String}
		aggregate := rollups[key]
		aggregate.Count++
		aggregate.DurationMillis += durationMillis
		aggregate.ToolCalls += toolCalls
		aggregate.CandidateCount += candidateCount
		aggregate.CostNanos += costNanos
		rollups[key] = aggregate
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("maintain quality ledger: close receipts: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("maintain quality ledger: iterate receipts: %w", err)
	}
	for key, aggregate := range rollups {
		_, err := tx.ExecContext(ctx, s.ph(`INSERT INTO quality_daily_aggregates
			(day, stage, outcome, run_snapshot_id, observation_count, duration_millis,
			 tool_calls, candidate_count, cost_nanos) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (day, stage, outcome, run_snapshot_id) DO UPDATE SET
			observation_count = quality_daily_aggregates.observation_count + excluded.observation_count,
			duration_millis = quality_daily_aggregates.duration_millis + excluded.duration_millis,
			tool_calls = quality_daily_aggregates.tool_calls + excluded.tool_calls,
			candidate_count = quality_daily_aggregates.candidate_count + excluded.candidate_count,
			cost_nanos = quality_daily_aggregates.cost_nanos + excluded.cost_nanos`),
			key.day, key.stage, key.outcome, key.snapshotID, aggregate.Count, aggregate.DurationMillis,
			aggregate.ToolCalls, aggregate.CandidateCount, aggregate.CostNanos)
		if err != nil {
			return fmt.Errorf("maintain quality ledger: upsert aggregate: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM quality_receipts WHERE occurred_at < ?`), receiptCutoff); err != nil {
		return fmt.Errorf("maintain quality ledger: purge receipts: %w", err)
	}
	aggregateCutoff := now.UTC().AddDate(-2, 0, 0).Format(time.DateOnly)
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM quality_daily_aggregates WHERE day < ?`), aggregateCutoff); err != nil {
		return fmt.Errorf("maintain quality ledger: purge aggregates: %w", err)
	}
	if err := s.pruneQualitySnapshots(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("maintain quality ledger: commit: %w", err)
	}
	return nil
}

func nullableQualitySnapshotID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func (s *sqlStore) pruneQualitySnapshots(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM quality_run_snapshots s
		WHERE NOT EXISTS (SELECT 1 FROM quality_receipts r WHERE r.run_snapshot_id = s.id)
		AND NOT EXISTS (SELECT 1 FROM quality_daily_aggregates a WHERE a.run_snapshot_id = s.id)
		ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return fmt.Errorf("maintain quality ledger: list unreferenced snapshots: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("maintain quality ledger: scan unreferenced snapshot: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("maintain quality ledger: close unreferenced snapshots: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("maintain quality ledger: iterate unreferenced snapshots: %w", err)
	}
	for _, id := range ids[min(256, len(ids)):] {
		if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM quality_run_snapshots WHERE id = ?`), id); err != nil {
			return fmt.Errorf("maintain quality ledger: purge snapshot: %w", err)
		}
	}
	return nil
}

func (s *sqlStore) ExportQualityLedger(ctx context.Context, now time.Time) (quality.Export, error) {
	out := quality.Export{
		SchemaVersion: qualityExportSchemaVersion,
		GeneratedAt:   now.UTC(),
		Aggregates:    make([]quality.Aggregate, 0),
		RunSnapshots:  make([]quality.RunSnapshot, 0),
	}
	rows, err := s.db.QueryContext(ctx, `SELECT day, stage, outcome, run_snapshot_id,
		observation_count, duration_millis, tool_calls, candidate_count, cost_nanos
		FROM quality_daily_aggregates ORDER BY day, stage, outcome, run_snapshot_id`)
	if err != nil {
		return quality.Export{}, fmt.Errorf("export quality ledger: list aggregates: %w", err)
	}
	for rows.Next() {
		var aggregate quality.Aggregate
		if err := rows.Scan(&aggregate.Day, &aggregate.Stage, &aggregate.Outcome, &aggregate.RunSnapshotID,
			&aggregate.Count, &aggregate.DurationMillis, &aggregate.ToolCalls, &aggregate.CandidateCount,
			&aggregate.CostNanos); err != nil {
			_ = rows.Close()
			return quality.Export{}, fmt.Errorf("export quality ledger: scan aggregate: %w", err)
		}
		out.Aggregates = append(out.Aggregates, aggregate)
	}
	if err := rows.Close(); err != nil {
		return quality.Export{}, fmt.Errorf("export quality ledger: close aggregates: %w", err)
	}
	if err := rows.Err(); err != nil {
		return quality.Export{}, fmt.Errorf("export quality ledger: iterate aggregates: %w", err)
	}

	rows, err = s.db.QueryContext(ctx, `SELECT id, schema_version, corpus_version, requested_model,
		resolved_model, provider, budget_profile, application_version, accounting_available, created_at
		FROM quality_run_snapshots s WHERE EXISTS
		(SELECT 1 FROM quality_daily_aggregates a WHERE a.run_snapshot_id = s.id)
		ORDER BY created_at, id`)
	if err != nil {
		return quality.Export{}, fmt.Errorf("export quality ledger: list snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		snapshot, err := scanQualityRunSnapshot(rows)
		if err != nil {
			return quality.Export{}, fmt.Errorf("export quality ledger: scan snapshot: %w", err)
		}
		out.RunSnapshots = append(out.RunSnapshots, snapshot)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(...any) error
}

func (s *sqlStore) getQualityRunSnapshot(ctx context.Context, q qualityRowQueryer, id string) (quality.RunSnapshot, error) {
	row := q.QueryRowContext(ctx, s.ph(`SELECT id, schema_version, corpus_version, requested_model,
		resolved_model, provider, budget_profile, application_version, accounting_available, created_at
		FROM quality_run_snapshots WHERE id = ?`), id)
	snapshot, err := scanQualityRunSnapshot(row)
	if errors.Is(err, sql.ErrNoRows) {
		return quality.RunSnapshot{}, ErrNotFound
	}
	return snapshot, err
}

type qualityRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func scanQualityRunSnapshot(row rowScanner) (quality.RunSnapshot, error) {
	var snapshot quality.RunSnapshot
	var provider string
	var createdAt int64
	err := row.Scan(&snapshot.ID, &snapshot.SchemaVersion, &snapshot.CorpusVersion, &snapshot.RequestedModel,
		&snapshot.ResolvedModel, &provider, &snapshot.BudgetProfile, &snapshot.ApplicationVersion,
		&snapshot.AccountingAvailable, &createdAt)
	snapshot.Provider = quality.Provider(provider)
	snapshot.CreatedAt = time.Unix(createdAt, 0).UTC()
	return snapshot, err
}
