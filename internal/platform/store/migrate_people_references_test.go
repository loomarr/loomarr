package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// peopleReferencesMigration makes a destination's owner, an invitation's account and a pull's
// decider foreign keys to users (#2793).
const peopleReferencesMigration = 224

func TestPeopleReferencesMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "people-references.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testPeopleReferencesMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testPeopleReferencesMigration proves the migration keeps every link to a person who exists,
// turns ” and a link to a person who is gone into NULL (or drops a destination that belonged to
// one), keeps an invitation's grants, and that afterwards the links are enforced: deleting a person
// takes their destinations and forgets them on invitations and pulls.
func testPeopleReferencesMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, peopleReferencesMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", peopleReferencesMigration-1, err)
	}
	const (
		destination = `INSERT INTO notification_destinations (id, means, label, scope, owner_id, audience,
			topics_json, enabled, credentials_encrypted, created_at, updated_at) VALUES `
		invitation = `INSERT INTO invitations (id, kind, username, identity_key, role, status, created_at,
			expires_at, terminal_at, redeemed_by) VALUES `
		grantHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	for _, stmt := range []string{
		`INSERT INTO users (id, name) VALUES ('90000951', 'Kept')`,
		destination + `('dest-person', 'ntfy', 'Mine', 'person', '90000951', 'person', '[]', TRUE, 'x', 1, 1),
			('dest-gone', 'ntfy', 'Theirs', 'person', '90000952', 'person', '[]', TRUE, 'x', 1, 1),
			('dest-house', 'ntfy', 'House', 'installation', '', 'operators', '[]', TRUE, 'x', 1, 1)`,
		invitation + `('inv-kept', 'local', 'kept', 'kept', 'member', 'redeemed', 1, 604801, 5, '90000951'),
			('inv-gone', 'local', 'gone', 'gone', 'member', 'redeemed', 1, 604801, 5, '90000953'),
			('inv-pending', 'local', 'new', 'new', 'member', 'pending', 1, 604801, 0, '')`,
		`INSERT INTO invitation_grants (token_hash, invitation_id, grant_kind, conveyance, created_at, expires_at)
			VALUES ('` + grantHash + `', 'inv-pending', 'activation', 'copy', 1, 2)`,
		`INSERT INTO filler_pulls (id, title, created_at, decided_by) VALUES
			('pull-kept', 'Kept', 1, '90000951'), ('pull-gone', 'Gone', 1, '90000954'), ('pull-open', 'Open', 1, '')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", peopleReferencesMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, peopleReferencesMigration); err != nil {
		t.Fatalf("apply %d: %v", peopleReferencesMigration, err)
	}

	link := func(query string) sql.NullString {
		t.Helper()
		var v sql.NullString
		if err := db.QueryRowContext(ctx, query).Scan(&v); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return v
	}
	count := func(query string) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n
	}
	for _, c := range []struct {
		what string
		got  sql.NullString
		want string // "" means NULL
	}{
		{"person's destination", link(`SELECT owner_id FROM notification_destinations WHERE id = 'dest-person'`), "90000951"},
		{"household destination", link(`SELECT owner_id FROM notification_destinations WHERE id = 'dest-house'`), ""},
		{"redeemed invitation", link(`SELECT redeemed_by FROM invitations WHERE id = 'inv-kept'`), "90000951"},
		{"invitation of a gone account", link(`SELECT redeemed_by FROM invitations WHERE id = 'inv-gone'`), ""},
		{"pending invitation", link(`SELECT redeemed_by FROM invitations WHERE id = 'inv-pending'`), ""},
		{"decided pull", link(`SELECT decided_by FROM filler_pulls WHERE id = 'pull-kept'`), "90000951"},
		{"pull of a gone admin", link(`SELECT decided_by FROM filler_pulls WHERE id = 'pull-gone'`), ""},
		{"open pull", link(`SELECT decided_by FROM filler_pulls WHERE id = 'pull-open'`), ""},
	} {
		if c.got.Valid != (c.want != "") || c.got.String != c.want {
			t.Errorf("%s = %v, want %q (\"\" is NULL)", c.what, c.got, c.want)
		}
	}
	if n := count(`SELECT count(*) FROM notification_destinations WHERE id = 'dest-gone'`); n != 0 {
		t.Errorf("a destination of a person who is gone survived the migration")
	}
	if n := count(`SELECT count(*) FROM invitation_grants WHERE invitation_id = 'inv-pending'`); n != 1 {
		t.Errorf("pending invitation has %d grants after the migration, want 1", n)
	}

	// Enforced from now on.
	for _, stmt := range []string{
		destination + `('dest-x', 'ntfy', 'X', 'person', '90000955', 'person', '[]', TRUE, 'x', 1, 1)`,
		destination + `('dest-y', 'ntfy', 'Y', 'person', NULL, 'person', '[]', TRUE, 'x', 1, 1)`,
		invitation + `('inv-x', 'local', 'x', 'x', 'member', 'redeemed', 1, 604801, 5, '90000956')`,
		invitation + `('inv-y', 'local', 'y', 'y', 'member', 'pending', 1, 604801, 0, '90000951')`,
		`INSERT INTO filler_pulls (id, title, created_at, decided_by) VALUES ('pull-x', 'X', 1, '90000957')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err == nil {
			t.Errorf("%s: accepted, want it refused", stmt)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = '90000951'`); err != nil {
		t.Fatalf("delete person: %v", err)
	}
	if n := count(`SELECT count(*) FROM notification_destinations WHERE id = 'dest-person'`); n != 0 {
		t.Errorf("a deleted person's destination survived them")
	}
	for _, c := range []struct {
		what string
		got  sql.NullString
	}{
		{"invitation after its account was deleted", link(`SELECT redeemed_by FROM invitations WHERE id = 'inv-kept'`)},
		{"pull after its admin was deleted", link(`SELECT decided_by FROM filler_pulls WHERE id = 'pull-kept'`)},
	} {
		if c.got.Valid {
			t.Errorf("%s = %q, want NULL", c.what, c.got.String)
		}
	}
}
