package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// Migration 00131 repairs the clip and pipeline rows an applied admission decision stamped in Unix
// nanoseconds, and leaves every Unix-seconds value as it was.
func TestDecisionClipTimestampsMigrationSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, sqliteFileAt(t, 130, storeConnections))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	provider, err := newMigrationProvider(s.db, s.dialect, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 130); err != nil {
		t.Fatalf("migrate through 130: %v", err)
	}
	decidedAt := time.Date(2026, 9, 13, 2, 1, 0, 0, time.UTC)
	untouchedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for _, hash := range []string{"decided", "untouched"} {
		// Seeded through the schema as it stood at 00130: current UpsertClip writes later columns.
		if _, err := s.db.ExecContext(ctx, `INSERT INTO clips (hash, path, name, kind, duration_ms, created_at, updated_at)
			VALUES (?, ?, ?, ?, 0, ?, ?)`, hash, hash+".mp4", hash, string(clips.Commercial),
			untouchedAt.Unix(), untouchedAt.Unix()); err != nil {
			t.Fatal(err)
		}
		if err := insertLegacyClipPipeline(ctx, s, pipelinestate.ClipPipeline{ClipHash: hash, Stage: pipelinestate.StageID("admission"),
			Status: pipelinestate.StatusDone, Disposition: pipelinestate.DispositionDismissed, UpdatedAt: untouchedAt}); err != nil {
			t.Fatal(err)
		}
	}
	// What an applied reject wrote before the fix.
	nanos := decidedAt.UnixNano()
	if _, err := s.db.ExecContext(ctx, `UPDATE clips SET updated_at = ?, removed_at = ? WHERE hash = 'decided'`, nanos, nanos); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE filler_clip_pipeline SET updated_at = ? WHERE clip_hash = 'decided'`, nanos); err != nil {
		t.Fatal(err)
	}

	// Only through 00131: replaying every later migration under -race costs the store lane minutes
	// (#2716), and TestSQLiteStoreCloneMatchesFreshReplay already covers the tail. The rows are read
	// back as raw Unix seconds, the 00131 schema's own representation.
	if _, err := provider.UpTo(ctx, 131); err != nil {
		t.Fatalf("apply decision timestamp repair: %v", err)
	}
	for hash, want := range map[string]struct{ updated, removed int64 }{
		"decided":   {decidedAt.Unix(), decidedAt.Unix()},
		"untouched": {untouchedAt.Unix(), 0},
	} {
		var clipUpdated, pipelineUpdated int64
		var clipRemoved sql.NullInt64
		if err := s.db.QueryRowContext(ctx, `SELECT updated_at, removed_at FROM clips WHERE hash = ?`, hash).
			Scan(&clipUpdated, &clipRemoved); err != nil {
			t.Fatal(err)
		}
		if err := s.db.QueryRowContext(ctx, `SELECT updated_at FROM filler_clip_pipeline WHERE clip_hash = ?`, hash).
			Scan(&pipelineUpdated); err != nil {
			t.Fatal(err)
		}
		if clipUpdated != want.updated || clipRemoved.Int64 != want.removed || pipelineUpdated != want.updated {
			t.Fatalf("%s after repair: clip updated %d removed %v, pipeline updated %d; want %d, %d",
				hash, clipUpdated, clipRemoved, pipelineUpdated, want.updated, want.removed)
		}
	}
}
