package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// crashDeviceMigration makes a crash report's device a foreign key to device_tokens (#2793).
const crashDeviceMigration = 239

func TestCrashDeviceMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "crash-device.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testCrashDeviceMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testCrashDeviceMigration proves the migration keeps a paired device, turns an empty one and a device
// already unpaired into NULL, and that afterwards the device is enforced and unpairing it clears
// it from the reports it sent, which stay.
func testCrashDeviceMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, crashDeviceMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", crashDeviceMigration-1, err)
	}
	report := func(id, device string) string {
		return `INSERT INTO client_crash_reports (id, received_at_ms, occurred_at_ms, device_id, device_name, kind,
			platform, app_version, app_build, os_version, device_model, message)
			VALUES ('` + id + `', 1, 1, '` + device + `', 'Phone', 'native', 'ios', '1', '1', '18', 'iPhone', 'boom')`
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, name) VALUES ('u-kid', 'Kid')`,
		`INSERT INTO device_tokens (token_hash, user_id, device_name) VALUES ('hash-paired', 'u-kid', 'Phone')`,
		report("crash-paired", "hash-paired"),
		report("crash-browser", ""),
		report("crash-unpaired", "hash-unpaired"),
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", crashDeviceMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, crashDeviceMigration); err != nil {
		t.Fatalf("apply %d: %v", crashDeviceMigration, err)
	}

	device := func(id string) string {
		t.Helper()
		var v sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT device_id FROM client_crash_reports WHERE id = '`+id+`'`).Scan(&v); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if !v.Valid {
			return "NULL"
		}
		return v.String
	}
	for id, want := range map[string]string{"crash-paired": "hash-paired", "crash-browser": "NULL", "crash-unpaired": "NULL"} {
		if got := device(id); got != want {
			t.Errorf("%s device = %s, want %s", id, got, want)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE client_crash_reports SET device_id = 'hash-none' WHERE id = 'crash-browser'`); err == nil {
		t.Error("accepted a report naming a device that is not paired")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM device_tokens WHERE token_hash = 'hash-paired'`); err != nil {
		t.Fatalf("unpair: %v", err)
	}
	if got := device("crash-paired"); got != "NULL" {
		t.Errorf("after unpairing, crash-paired device = %s, want NULL", got)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM client_crash_reports`).Scan(&n); err != nil || n != 3 {
		t.Errorf("reports = %d, %v; want all 3 kept", n, err)
	}
}
