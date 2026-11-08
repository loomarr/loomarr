package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// foreignKeyPolicyMigration is the migration that brings two foreign keys into line with the
// storage.md policy (#2323).
const foreignKeyPolicyMigration = 167

func TestForeignKeyPolicyMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "foreign-keys.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testForeignKeyPolicyMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testForeignKeyPolicyMigration proves the migration prunes airings of channels already deleted,
// keeps everything else, and that afterwards deleting a user keeps their crash report while
// deleting a channel takes its airings with it.
func testForeignKeyPolicyMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, foreignKeyPolicyMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", foreignKeyPolicyMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, name) VALUES ('kid', 'Kid')`,
		`INSERT INTO channels (id, name, number, strategy, status) VALUES ('ch-live', 'Live', 1, 'shuffle', 'live')`,
		`INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at) VALUES ('ch-live', 'ep-1', 10, 10)`,
		`INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at) VALUES ('ch-gone', 'ep-1', 10, 10)`,
		`INSERT INTO client_crash_reports (id, received_at_ms, occurred_at_ms, user_id, kind, platform, app_version,
			app_build, os_version, device_model, message)
			VALUES ('crash-1', 1, 1, 'kid', 'native', 'ios', '1', '1', '18', 'iPhone', 'boom')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", foreignKeyPolicyMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, foreignKeyPolicyMigration); err != nil {
		t.Fatalf("apply %d: %v", foreignKeyPolicyMigration, err)
	}

	count := func(query string) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM airings WHERE channel_id = 'ch-gone'`); n != 0 {
		t.Errorf("%d airings of a deleted channel survived the migration, want them pruned", n)
	}
	if n := count(`SELECT count(*) FROM airings WHERE channel_id = 'ch-live'`); n != 1 {
		t.Errorf("live channel has %d airings, want its airing kept", n)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO airings (channel_id, library_item_id, aired_at, recorded_at)
		VALUES ('ch-never', 'ep-1', 10, 10)`); err == nil {
		t.Error("an airing for a channel that does not exist was accepted; the foreign key is missing")
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = 'kid'`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var userID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT user_id FROM client_crash_reports WHERE id = 'crash-1'`).Scan(&userID); err != nil {
		t.Fatalf("crash report after its user was deleted: %v", err)
	}
	if userID.Valid {
		t.Errorf("crash report user_id = %q after the user was deleted, want NULL", userID.String)
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM channels WHERE id = 'ch-live'`); err != nil {
		t.Fatalf("delete channel: %v", err)
	}
	if n := count(`SELECT count(*) FROM airings WHERE channel_id = 'ch-live'`); n != 0 {
		t.Errorf("deleting a channel left %d airings, want them cascaded", n)
	}
}
