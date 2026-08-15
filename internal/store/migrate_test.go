package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestDowngradeGuard: if the DB records a schema version newer than the binary
// embeds, Open must refuse (§16) instead of limping into an unknown schema.
func TestDowngradeGuard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.db")

	// Open+migrate normally so goose's version table exists and is current.
	s, err := Open(context.Background(), "sqlite://"+path, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	// Forge a from-the-future version row (higher than any embedded migration).
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO goose_db_version (version_id, is_applied) VALUES (99999, 1)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Re-open: the guard must fire.
	_, err = Open(context.Background(), "sqlite://"+path, true)
	if err == nil {
		t.Fatal("expected downgrade guard to refuse a from-the-future DB")
	}
}

func TestMigrationProviderIsDialectLocal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}

	// Changing the legacy process-global dialect must not affect this provider.
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		t.Fatalf("migrate SQLite after changing legacy dialect: %v", err)
	}

	got, err := provider.GetDBVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := highestMigration("migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}
}

func TestMigrationProviderScopesEmbeddedMigrations(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "sources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	sources := provider.ListSources()
	if len(sources) == 0 {
		t.Fatal("provider has no migration sources")
	}

	want, err := highestMigration("migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got := sources[len(sources)-1].Version; got != want {
		t.Fatalf("highest provider source = %d, want %d", got, want)
	}
}

func TestUniqueChannelIntentRefMigrationPreservesDuplicateChannels(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "intent-ref.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
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
			 VALUES (?, ?, ?, ?, 'sequential', 'building', ?)`,
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

func TestChannelRevisionMigrationBackfillsExistingRows(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "channel-revision.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
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

func TestMigrationProviderRejectsUnknownDialect(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "unknown.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err := newMigrationProvider(db, Dialect("unknown"), "migrations/sqlite"); err == nil {
		t.Fatal("expected an unknown dialect error")
	}
}
