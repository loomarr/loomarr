package store

import (
	"context"
	"path/filepath"
	"testing"
)

// The attempts migration rebuilds the window call journal: an existing settled row must survive as
// attempt 1, and the new key must admit a second attempt of the same request digest.
func TestStructureWindowCallAttemptsMigrationKeepsRowsAsAttemptOneSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "window-attempts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	provider, err := newMigrationProvider(s.db, s.dialect, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 144); err != nil {
		t.Fatalf("migrate through 144: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO filler_inference_evaluations
		(id, cache_key, clip_hash, role, state, requested_provider, requested_model, evidence_version,
		 extractor_version, prompt_version, schema_version, taxonomy_version, admission_policy_version,
		 role_policy_version, capability_snapshot, created_at, updated_at)
		VALUES ('evaluation-1', 'key', 'clip', 'role', 'failed', 'p', 'm', 'e', 'x', 'p', 's', 't', 'a', 'r', 'c', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO filler_structure_window_call_ledger
		(request_sha256, evaluation_id, source_sha256, media_set_sha256, window_ordinal, assessor_id, state,
		 reservation_json, record_sha256, record_json, requested_at, assessed_at)
		VALUES ('request', 'evaluation-1', 'source', 'set', 0, 'assessor', 'settled', '{}', 'record', '{}', 1, 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 146); err != nil {
		t.Fatalf("apply window call attempts: %v", err)
	}
	var attempt int
	var state string
	if err := s.db.QueryRowContext(ctx, `SELECT attempt, state FROM filler_structure_window_call_ledger WHERE request_sha256 = 'request'`).Scan(&attempt, &state); err != nil {
		t.Fatal(err)
	}
	if attempt != 1 || state != "settled" {
		t.Fatalf("migrated row attempt=%d state=%q, want attempt 1 settled", attempt, state)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO filler_inference_evaluations
		(id, cache_key, clip_hash, role, state, requested_provider, requested_model, evidence_version,
		 extractor_version, prompt_version, schema_version, taxonomy_version, admission_policy_version,
		 role_policy_version, capability_snapshot, created_at, updated_at)
		VALUES ('evaluation-2', 'key', 'clip', 'role', 'reserved', 'p', 'm', 'e', 'x', 'p', 's', 't', 'a', 'r', 'c', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO filler_structure_window_call_ledger
		(request_sha256, attempt, evaluation_id, source_sha256, media_set_sha256, window_ordinal, assessor_id, state,
		 reservation_json, requested_at)
		VALUES ('request', 2, 'evaluation-2', 'source', 'set', 0, 'assessor', 'open', '{}', 3)`); err != nil {
		t.Fatalf("second attempt of the same request digest: %v", err)
	}
}
