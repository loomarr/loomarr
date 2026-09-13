package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestRetireFillerRuntimeRightsMigrationSQLite(t *testing.T) {
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "retire-runtime-rights.db"))
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
	if err := s.UpsertClip(ctx, Clip{Clip: filler.Clip{
		Hash: hash, Path: "aa/legacy.mp4", Name: "Legacy admission", Kind: filler.Commercial,
		DurationMs: 30_000, Held: false,
	}, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClipPipeline(ctx, filler.ClipPipeline{
		ClipHash: hash, Stage: filler.StageAdmission, Status: filler.StatusDone, Progress: 100,
		Disposition: filler.DispositionFiled, EnrolledAt: at, UpdatedAt: at,
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

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("apply runtime-rights retirement: %v", err)
	}

	clip, err := s.GetClip(ctx, hash)
	if err != nil || !clip.Held {
		t.Fatalf("affected clip = %+v, err = %v; want held", clip, err)
	}
	if _, found, err := s.GetClipPipeline(ctx, hash); err != nil || found {
		t.Fatalf("affected pipeline found = %t, err = %v; want re-enrolment", found, err)
	}
	for _, table := range []string{
		"filler_admission_decisions", "filler_admission_actions", "filler_diagnostic_recovery_actions",
	} {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s rows = %d, err = %v; want retired rows removed", table, count, err)
		}
	}
	for _, table := range []string{"filler_rights_heads", "filler_rights_grants"} {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err == nil || err == sql.ErrNoRows {
			t.Fatalf("retired table %s remains queryable", table)
		}
	}
}
