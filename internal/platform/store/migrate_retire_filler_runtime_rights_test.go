package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

func TestRetireFillerRuntimeRightsMigrationSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, sqliteFileAt(t, 105, storeConnections))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testRetireFillerRuntimeRightsMigration(t, s, "migrations/sqlite")
}

func testRetireFillerRuntimeRightsMigration(t *testing.T, s *sqlStore, migrationDir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(s.db, s.dialect, migrationDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 105); err != nil {
		t.Fatalf("migrate through 105: %v", err)
	}

	at := time.Date(2026, time.September, 13, 12, 0, 0, 0, time.UTC)
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// Seed through the schema as it existed before 00106/00107. Current UpsertClip intentionally
	// speaks the current placement column and must not pretend it can write an older schema.
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO clips
		(hash, path, name, kind, duration_ms, held, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`), hash, "aa/legacy.mp4", "Legacy admission",
		string(clips.Commercial), 30_000, false, at.Unix()); err != nil {
		t.Fatal(err)
	}
	if err := insertLegacyClipPipeline(ctx, s, pipelinestate.ClipPipeline{
		ClipHash: hash, Stage: pipelinestate.StageID("admission"), Status: pipelinestate.StatusDone, Progress: 100,
		Disposition: pipelinestate.Disposition("filed"), EnrolledAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO filler_admission_decisions (
		id, clip_hash, evidence_hash, evidence_version, schema_version, policy_version,
		taxonomy_version, outcome_kind, verdict, result_json, created_at
	) VALUES (?, ?, 'old-evidence', 'old-v1', 2, 'old-policy', 'old-taxonomy',
		'semantic', 'review', '{}', ?)`), "old-decision", hash, at.UnixNano()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO filler_admission_actions
		(id, decision_id, kind, actor_id, created_at) VALUES ('old-action', 'old-decision', 'abandon', 'admin', ?)`), at.UnixNano()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO filler_diagnostic_recovery_actions
		(id, decision_id, action, actor_id, created_at) VALUES ('old-recovery', 'old-decision', 'retry', 'admin', ?)`), at.UnixNano()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO filler_rights_grants (
		grant_sha256, source_id, acquisition_id, source_master_sha256, policy_sha256,
		use_name, grant_json, recorded_at
	) VALUES ('grant', 'source', 'acquisition', 'master', 'policy', 'filler_broadcast', '{}', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO filler_rights_heads (
		source_id, acquisition_id, source_master_sha256, policy_sha256, use_name, grant_sha256
	) VALUES ('source', 'acquisition', 'master', 'policy', 'filler_broadcast', 'grant')`); err != nil {
		t.Fatal(err)
	}

	// Only through 00106: replaying every later migration under -race costs the store lane minutes
	// (#2716), and TestSQLiteStoreCloneMatchesFreshReplay already covers the tail.
	if _, err := provider.UpTo(ctx, 106); err != nil {
		t.Fatalf("apply runtime-rights retirement: %v", err)
	}

	var held bool
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT held FROM clips WHERE hash = ?`), hash).Scan(&held); err != nil || !held {
		t.Fatalf("affected clip held = %t, err = %v; want held", held, err)
	}
	var pipelines int
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT COUNT(*) FROM filler_clip_pipeline WHERE clip_hash = ?`), hash).
		Scan(&pipelines); err != nil || pipelines != 0 {
		t.Fatalf("affected pipeline rows = %d, err = %v; want re-enrolment", pipelines, err)
	}
	for _, table := range []string{"filler_rights_heads", "filler_rights_grants"} {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err == nil || err == sql.ErrNoRows {
			t.Fatalf("retired table %s remains queryable", table)
		}
	}
	// 00106 empties the superseded admission history; a later migration drops its tables.
	for _, table := range []string{
		"filler_admission_decisions", "filler_admission_actions", "filler_diagnostic_recovery_actions",
	} {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("superseded %s rows = %d, err = %v; want none", table, count, err)
		}
	}
}
