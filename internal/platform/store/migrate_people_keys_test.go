package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// peopleKeysMigration makes the last columns naming a member foreign keys to users (#2793).
const peopleKeysMigration = 234

func TestPeopleKeysMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "people-keys.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testPeopleKeysMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testPeopleKeysMigration proves the migration keeps each member, turns 'system', the empty id and
// an id naming no user into NULL, drops a personal destination of no member, and that afterwards
// deleting a member clears who changed a setting, redeemed an invitation or decided a pull, and
// takes their personal destinations.
func testPeopleKeysMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, peopleKeysMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", peopleKeysMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, name) VALUES ('u-admin', 'Admin'), ('u-kid', 'Kid')`,
		`INSERT INTO settings (key, value, updated_by) VALUES
			('s-admin', 'v', 'u-admin'), ('s-system', 'v', 'system'), ('s-empty', 'v', ''),
			('s-gone', 'v', 'u-gone'), ('s-null', 'v', NULL)`,
		`INSERT INTO filler_pulls (id, title, created_at, status, decided_by) VALUES
			('pull-decided', 'p', 1, 'approved', 'u-admin'), ('pull-pending', 'p', 1, 'pending', '')`,
		`INSERT INTO invitations (id, kind, username, identity_key, role, status, created_at, expires_at,
			terminal_at, redeemed_by) VALUES
			('inv-redeemed', 'local', 'kid', 'kid', 'member', 'redeemed', 1, 604801, 5, 'u-kid'),
			('inv-pending', 'local', 'nan', 'nan', 'member', 'pending', 1, 604801, 0, '')`,
		`INSERT INTO invitation_grants (token_hash, invitation_id, grant_kind, conveyance, created_at, expires_at)
			VALUES ('` + hex64 + `', 'inv-pending', 'activation', 'copy', 1, 2)`,
		`INSERT INTO notification_destinations (id, means, label, scope, owner_id, audience, topics_json,
			enabled, credentials_encrypted, created_at, updated_at) VALUES
			('d-house', 'slack', 'House', 'installation', '', 'operators', '[]', TRUE, '', 1, 1),
			('d-kid', 'slack', 'Kid', 'person', 'u-kid', 'person', '[]', TRUE, '', 1, 1),
			('d-gone', 'slack', 'Gone', 'person', 'u-gone', 'person', '[]', TRUE, '', 1, 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", peopleKeysMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, peopleKeysMigration); err != nil {
		t.Fatalf("apply %d: %v", peopleKeysMigration, err)
	}

	value := func(query string) string {
		t.Helper()
		var v sql.NullString
		if err := db.QueryRowContext(ctx, query).Scan(&v); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if !v.Valid {
			return "NULL"
		}
		return v.String
	}
	check := func(want map[string]string) {
		t.Helper()
		for query, want := range want {
			if got := value(query); got != want {
				t.Errorf("%s = %s, want %s", query, got, want)
			}
		}
	}
	check(map[string]string{
		`SELECT updated_by FROM settings WHERE key = 's-admin'`:               "u-admin",
		`SELECT count(*) FROM settings WHERE updated_by IS NULL`:              "4",
		`SELECT decided_by FROM filler_pulls WHERE id = 'pull-decided'`:       "u-admin",
		`SELECT decided_by FROM filler_pulls WHERE id = 'pull-pending'`:       "NULL",
		`SELECT redeemed_by FROM invitations WHERE id = 'inv-redeemed'`:       "u-kid",
		`SELECT redeemed_by FROM invitations WHERE id = 'inv-pending'`:        "NULL",
		`SELECT count(*) FROM invitation_grants`:                              "1",
		`SELECT owner_id FROM notification_destinations WHERE id = 'd-house'`: "NULL",
		`SELECT owner_id FROM notification_destinations WHERE id = 'd-kid'`:   "u-kid",
		`SELECT count(*) FROM notification_destinations WHERE id = 'd-gone'`:  "0",
	})

	for _, stmt := range []string{
		`UPDATE settings SET updated_by = 'u-none' WHERE key = 's-admin'`,
		`UPDATE filler_pulls SET decided_by = 'u-none' WHERE id = 'pull-decided'`,
		`UPDATE invitations SET redeemed_by = 'u-none' WHERE id = 'inv-redeemed'`,
		`UPDATE invitations SET redeemed_by = 'u-kid' WHERE id = 'inv-pending'`,
		`UPDATE notification_destinations SET owner_id = 'u-none' WHERE id = 'd-kid'`,
		`UPDATE notification_destinations SET owner_id = 'u-kid' WHERE id = 'd-house'`,
		`UPDATE notification_destinations SET owner_id = NULL WHERE id = 'd-kid'`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("accepted a person who is no member, or broke a scope's owner rule: %s", stmt)
		}
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id IN ('u-admin', 'u-kid')`); err != nil {
		t.Fatalf("delete members: %v", err)
	}
	check(map[string]string{
		`SELECT updated_by FROM settings WHERE key = 's-admin'`:               "NULL",
		`SELECT decided_by FROM filler_pulls WHERE id = 'pull-decided'`:       "NULL",
		`SELECT status FROM invitations WHERE id = 'inv-redeemed'`:            "redeemed",
		`SELECT redeemed_by FROM invitations WHERE id = 'inv-redeemed'`:       "NULL",
		`SELECT count(*) FROM notification_destinations WHERE id = 'd-kid'`:   "0",
		`SELECT count(*) FROM notification_destinations WHERE id = 'd-house'`: "1",
	})
}

// hex64 is a valid invitation grant token hash.
const hex64 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
