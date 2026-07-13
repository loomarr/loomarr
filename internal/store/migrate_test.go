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
	if err := goose.SetDialect("sqlite"); err != nil {
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
