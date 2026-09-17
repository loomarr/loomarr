//go:build integration

package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestPostgresDiscoveryFeedbackOrphanCleanupMigration(t *testing.T) {
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testDiscoveryFeedbackOrphanCleanupMigration(t, db, DialectPostgres, "migrations/postgres")
}

func TestPostgresInvitationMigrationPreservesExistingContactAddresses(t *testing.T) {
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testInvitationMigrationPreservesExistingContactAddresses(t, db, DialectPostgres, "migrations/postgres")
}

func TestPostgresQuarantineLegacyFillerMigration(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)
	s, err := openPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	provider, err := newMigrationProvider(s.db, DialectPostgres, "migrations/postgres")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	testQuarantineLegacyFillerMigration(t, s, s.db, "migrations/postgres/00101_quarantine_legacy_filler.sql")
}

func TestPostgresRetireFillerRuntimeRightsMigration(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)
	s, err := openPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	testRetireFillerRuntimeRightsMigration(t, s, "migrations/postgres")
}

func TestPostgresFillerReadyDispositionsMigration(t *testing.T) {
	ctx := context.Background()
	s, err := openPostgres(ctx, startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	testFillerReadyDispositionsMigration(t, s, "migrations/postgres")
}

func TestPostgresRemoveFillerAdmissionRungMigration(t *testing.T) {
	ctx := context.Background()
	s, err := openPostgres(ctx, startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	testRemoveFillerAdmissionRungMigration(t, s, "migrations/postgres")
}

func TestPostgresFillerStorageGovernorMigration(t *testing.T) {
	ctx := context.Background()
	s, err := openPostgres(ctx, startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	provider, err := newMigrationProvider(s.db, DialectPostgres, "migrations/postgres")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 111); err != nil {
		t.Fatalf("migrate through 111: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO settings
		(key, value, updated_at, updated_by, env_override) VALUES ($1, $2, $3, $4, $5)`,
		"filler.fetch.max_disk_gb", "37", int64(1234), "admin-1", false); err != nil { // retired-ok: migration fixture
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 112); err != nil {
		t.Fatalf("apply storage governor migration: %v", err)
	}
	var value, updatedBy string
	var updatedAt int64
	var envOverride bool
	if err := s.db.QueryRowContext(ctx, `SELECT value, updated_at, updated_by, env_override
		FROM settings WHERE key = $1`, "filler.storage.library_budget_gb").Scan(
		&value, &updatedAt, &updatedBy, &envOverride,
	); err != nil {
		t.Fatal(err)
	}
	if value != "37" || updatedAt != 1234 || updatedBy != "admin-1" || envOverride {
		t.Fatalf("migrated setting = value %q updated_at %d updated_by %q env_override %t", value, updatedAt, updatedBy, envOverride)
	}
	assertSettingAbsent(t, ctx, s, "filler.fetch.max_disk_gb") // retired-ok: migration assertion
}

func TestPostgresFillerDecisionApplicationModeMigrationBackfillsShadow(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectPostgres, "migrations/postgres")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 70); err != nil {
		t.Fatalf("migrate through 70: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO filler_admission_decisions (
		id, clip_hash, evidence_hash, evidence_version, schema_version, policy_version,
		taxonomy_version, outcome_kind, verdict, result_json, created_at
	) VALUES ('pre-mode', 'clip-1', 'evidence-1', 'e1', 2, 'p1', 't1',
		'semantic', 'admit', '{}', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 71); err != nil {
		t.Fatalf("migrate through 71: %v", err)
	}
	var mode string
	if err := db.QueryRowContext(ctx,
		`SELECT application_mode FROM filler_admission_decisions WHERE id = 'pre-mode'`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "shadow" {
		t.Fatalf("pre-migration application mode = %q, want shadow", mode)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE filler_admission_decisions SET application_mode = 'automatic' WHERE id = 'pre-mode'`); err == nil {
		t.Fatal("application mode CHECK accepted an unknown value")
	}
}

func TestPostgresUniqueChannelIntentRefMigrationPreservesDuplicateChannels(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectPostgres, "migrations/postgres")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 54); err != nil {
		t.Fatalf("migrate through 54: %v", err)
	}
	insert := func(id, intentRef string, number int, updatedAt int64) {
		t.Helper()
		if _, err := db.ExecContext(ctx,
			`INSERT INTO channels (id, intent_ref, name, number, strategy, status, updated_at)
			 VALUES ($1, $2, $3, $4, 'sequential', 'building', $5)`,
			id, intentRef, id, number, updatedAt); err != nil {
			t.Fatal(err)
		}
	}
	insert("older", "job-one", 1, 10)
	insert("newer", "job-one", 2, 20)
	insert("tie-a", "job-tie", 3, 30)
	insert("tie-z", "job-tie", 4, 30)
	insert("empty-a", "", 5, 40)
	insert("empty-b", "", 6, 50)
	insert("active-old", "job-detached", 7, 10)
	insert("detached-new", "job-detached", 8, 100)
	if _, err := db.ExecContext(ctx, `UPDATE channels SET status='detached' WHERE id='detached-new'`); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 55); err != nil {
		t.Fatalf("migrate through 55: %v", err)
	}
	refs := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT id, intent_ref FROM channels`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, ref string
		if err := rows.Scan(&id, &ref); err != nil {
			t.Fatal(err)
		}
		refs[id] = ref
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(refs) != 8 {
		t.Fatalf("channels after migration = %d, want all 8 preserved", len(refs))
	}
	if refs["newer"] != "job-one" || refs["older"] != "" {
		t.Errorf("updated_at winner refs = %+v", refs)
	}
	if refs["tie-z"] != "job-tie" || refs["tie-a"] != "" {
		t.Errorf("id tie-break winner refs = %+v", refs)
	}
	if refs["empty-a"] != "" || refs["empty-b"] != "" {
		t.Errorf("empty refs changed = %+v", refs)
	}
	if refs["active-old"] != "job-detached" || refs["detached-new"] != "" {
		t.Errorf("managed row did not win over newer detached duplicate: %+v", refs)
	}

	insert("empty-c", "", 9, 60)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO channels (id, intent_ref, name, number, strategy, status)
		 VALUES ('duplicate', 'job-one', 'duplicate', 10, 'sequential', 'building')`); err == nil {
		t.Fatal("partial unique index accepted a second non-empty intent_ref")
	}
}

func TestPostgresChannelRevisionMigrationBackfillsExistingRows(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("pgx", startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectPostgres, "migrations/postgres")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 55); err != nil {
		t.Fatalf("migrate through 55: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO channels (id, name, number, strategy, status)
		 VALUES ('pre-revision', 'Pre revision', 156, 'sequential', 'building')`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 56); err != nil {
		t.Fatalf("migrate through 56: %v", err)
	}
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT revision FROM channels WHERE id='pre-revision'`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 1 {
		t.Fatalf("backfilled revision = %d, want 1", revision)
	}
	if _, err := db.ExecContext(ctx, `UPDATE channels SET revision=0 WHERE id='pre-revision'`); err == nil {
		t.Fatal("channel revision CHECK accepted zero")
	}
}
