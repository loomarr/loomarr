package store

import (
	"context"
	"path/filepath"
	"testing"
)

// newSQLiteStore builds a fresh migrated SQLite store in a temp file per test.
// A file (not :memory:) is used because WAL + the single-conn model is what
// production runs; t.TempDir cleans it up.
func newSQLiteStore(t *testing.T) Store {
	t.Helper()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), dsn, true)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestSQLiteConformance runs the shared suite against SQLite. Phase 4 adds the
// identical call for Postgres.
func TestSQLiteConformance(t *testing.T) {
	RunConformance(t, newSQLiteStore)
}

func TestUnknownSchemeFailsFast(t *testing.T) {
	_, err := Open(context.Background(), "mysql://nope", false)
	if err == nil {
		t.Fatal("expected error for unknown DATABASE_URL scheme")
	}
}
