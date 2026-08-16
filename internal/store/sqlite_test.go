package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
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

func TestMigrationSourceIsStructurallyReadOnly(t *testing.T) {
	ctx := context.Background()
	live := newSQLiteStore(t)
	if err := live.SetSetting(ctx, "migration.probe", "before"); err != nil {
		t.Fatal(err)
	}

	snapshot, err := openSQLiteReadOnly(ctx, SQLitePath(live))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = snapshot.Close() }()

	if err := snapshot.SetSetting(ctx, "migration.probe", "after"); err == nil {
		t.Fatal("read-only migration source accepted a write")
	}
	if got, err := snapshot.GetSetting(ctx, "migration.probe"); err != nil || got != "before" {
		t.Fatalf("read-only source value = %q (err %v), want before", got, err)
	}
}

func TestOpenHealsPreAtomicTaxonomyProjections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "taxonomy-upgrade.db")
	dsn := "sqlite://" + path
	s, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	clip := sampleClip("upgrade-clip", "upgrade.mp4", filler.Commercial, 1994, filler.General, "")
	if err := s.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipTags(ctx, clip.Hash, []string{"cereal"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate the state an old disabled repair job or a manual restore could leave behind: the
	// asserted leaf is trustworthy, while its rollup, category shadow, and closure are stale.
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM clip_tags WHERE leaf = FALSE;
		UPDATE clips SET category = 'cars' WHERE hash = 'upgrade-clip';
		DELETE FROM taxa_closure WHERE ancestor = 'food' AND descendant = 'cereal'`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	got, err := reopened.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "cereal" {
		t.Errorf("healed category = %q, want cereal", got.Category)
	}
	assertSet(t, "healed tags", got.Tags, []string{"cereal", "food"})
}

func TestUnknownSchemeFailsFast(t *testing.T) {
	_, err := Open(context.Background(), "mysql://nope", false)
	if err == nil {
		t.Fatal("expected error for unknown DATABASE_URL scheme")
	}
}
