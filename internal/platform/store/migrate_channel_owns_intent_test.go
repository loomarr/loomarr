package store

import (
	"context"
	"path/filepath"
	"testing"
)

// Migration 00145 gives a channel its own intent and requester. It copies both once, from the job
// each channel's `intent_ref` still names; a channel whose job was already purged keeps empty
// values, and a hand-made channel (no `intent_ref`) is untouched.
func TestChannelOwnsIntentMigrationSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "channel-owns-intent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testChannelOwnsIntentMigration(t, s, "migrations/sqlite")
}

func testChannelOwnsIntentMigration(t *testing.T, s *sqlStore, migrationDir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(s.db, s.dialect, migrationDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 144); err != nil {
		t.Fatalf("migrate through 144: %v", err)
	}

	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO jobs
		(id, kind, status, intent_json, intent_hash, created_by, deadline, created_at, updated_at)
		VALUES (?, 'suggest', 'done', ?, 'h', 'u-member', 0, 1, 1)`),
		"job-alive", `{"description":"90s action","era":"1990s"}`); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	for _, c := range []struct {
		id, intentRef string
		number        int
	}{
		{"ch-alive", "job-alive", 1},
		{"ch-purged", "job-purged", 2},
		{"ch-hand", "", 3},
	} {
		if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO channels
			(id, intent_ref, name, number, strategy, status) VALUES (?, ?, ?, ?, 'sequential', 'live')`),
			c.id, c.intentRef, c.id, c.number); err != nil {
			t.Fatalf("seed channel %s: %v", c.id, err)
		}
	}

	if _, err := provider.UpTo(ctx, 145); err != nil {
		t.Fatalf("apply channel owns intent: %v", err)
	}
	for _, want := range []struct{ id, intent, requestedBy string }{
		{"ch-alive", `{"description":"90s action","era":"1990s"}`, "u-member"},
		{"ch-purged", "", ""},
		{"ch-hand", "", ""},
	} {
		var intent, requestedBy string
		if err := s.db.QueryRowContext(ctx, s.ph(
			`SELECT intent_json, requested_by FROM channels WHERE id = ?`), want.id).Scan(&intent, &requestedBy); err != nil {
			t.Fatal(err)
		}
		if intent != want.intent || requestedBy != want.requestedBy {
			t.Errorf("%s = intent %q requestedBy %q, want %q / %q", want.id, intent, requestedBy, want.intent, want.requestedBy)
		}
	}
}
