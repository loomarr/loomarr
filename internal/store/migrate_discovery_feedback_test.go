package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"
)

func TestDiscoveryFeedbackOrphanCleanupMigration(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "feedback-orphans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testDiscoveryFeedbackOrphanCleanupMigration(t, db, DialectSQLite, "migrations/sqlite")
}

func testDiscoveryFeedbackOrphanCleanupMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 71); err != nil {
		t.Fatalf("migrate through 71: %v", err)
	}
	ph := placeholder(passthrough)
	if dialect == DialectPostgres {
		ph = pgPlaceholders
	}
	for _, channel := range []struct {
		id     string
		number int
		status string
	}{
		{id: "active-channel", number: 41, status: "live"},
		{id: "detached-channel", number: 42, status: "detached"},
	} {
		if _, err := db.ExecContext(ctx, ph(`INSERT INTO channels (id, name, number, strategy, status)
			VALUES (?, ?, ?, 'sequential', ?)`), channel.id, channel.id, channel.number, channel.status); err != nil {
			t.Fatal(err)
		}
	}
	events := []struct {
		id      string
		scope   string
		scopeID string
	}{
		{id: "active-event", scope: "channel", scopeID: "active-channel"},
		{id: "detached-event", scope: "channel", scopeID: "detached-channel"},
		{id: "household-event", scope: "household", scopeID: ""},
		{id: "orphan-event", scope: "channel", scopeID: "purged-channel"},
	}
	for _, event := range events {
		query := `INSERT INTO discovery_feedback
			(id, actor_id, scope, scope_id, target_key, action, created_at)
			VALUES (?, 'admin', ?, ?, 'movie:tmdb:603', 'never', 1)`
		_, err := db.ExecContext(ctx, ph(query), event.id, event.scope, event.scopeID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := provider.UpTo(ctx, 72); err != nil {
		t.Fatalf("migrate through 72: %v", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM discovery_feedback ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"active-event", "detached-event", "household-event"}
	if !slices.Equal(got, want) {
		t.Fatalf("feedback after orphan cleanup = %v, want %v", got, want)
	}
}
