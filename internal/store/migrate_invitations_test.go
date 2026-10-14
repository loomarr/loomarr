package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestInvitationMigrationPreservesExistingContactAddresses(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "invitation-upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testInvitationMigrationPreservesExistingContactAddresses(t, db, DialectSQLite, "migrations/sqlite")
}

func testInvitationMigrationPreservesExistingContactAddresses(
	t *testing.T,
	db *sql.DB,
	dialect Dialect,
	dir string,
) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 77); err != nil {
		t.Fatalf("migrate through 77: %v", err)
	}
	ph := placeholder(passthrough)
	if dialect == DialectPostgres {
		ph = pgPlaceholders
	}
	if _, err := db.ExecContext(ctx, ph(`INSERT INTO users
		(id, name, role, password_hash, media_server_linked)
		VALUES (?, ?, 'member', NULL, ?)`), "existing", "Existing user", true); err != nil {
		t.Fatalf("seed existing user: %v", err)
	}
	if _, err := db.ExecContext(ctx, ph(`INSERT INTO user_contact_addresses
		(user_id, email, normalized, status, provenance, created_at, verified_at)
		VALUES (?, ?, ?, 'verified', 'admin', ?, ?)`),
		"existing", "Ada@Example.com", "ada@example.com", int64(100), int64(200)); err != nil {
		t.Fatalf("seed existing contact: %v", err)
	}
	if _, err := provider.UpTo(ctx, 78); err != nil {
		t.Fatalf("apply invitation migration: %v", err)
	}

	var ownerKind, ownerID, email, normalized, status, provenance string
	var createdAt, verifiedAt int64
	if err := db.QueryRowContext(ctx, `SELECT owner_kind, owner_id, email, normalized,
		status, provenance, created_at, verified_at FROM contact_addresses`).Scan(
		&ownerKind, &ownerID, &email, &normalized, &status, &provenance, &createdAt, &verifiedAt,
	); err != nil {
		t.Fatalf("read migrated contact: %v", err)
	}
	if ownerKind != "user" || ownerID != "existing" || email != "Ada@Example.com" ||
		normalized != "ada@example.com" || status != "verified" || provenance != "admin" ||
		createdAt != 100 || verifiedAt != 200 {
		t.Fatalf("migrated contact = %q %q %q %q %q %q %d %d",
			ownerKind, ownerID, email, normalized, status, provenance, createdAt, verifiedAt)
	}
	for _, table := range []string{"invitations", "invitation_grants"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("new %s rows = %d, want empty", table, count)
		}
	}
}
