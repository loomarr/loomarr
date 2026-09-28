package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// Migration 00131 repairs the clip and pipeline rows an applied admission decision stamped in Unix
// nanoseconds, and leaves every Unix-seconds value as it was.
func TestDecisionClipTimestampsMigrationSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "decision-timestamps.db"))
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
		if err := s.UpsertClip(ctx, Clip{Clip: filler.Clip{Hash: hash, Path: hash + ".mp4", Name: hash,
			Kind: filler.Commercial}, UpdatedAt: untouchedAt}); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertClipPipeline(ctx, filler.ClipPipeline{ClipHash: hash, Stage: filler.StageID("admission"),
			Status: filler.StatusDone, Disposition: filler.DispositionDismissed, UpdatedAt: untouchedAt}); err != nil {
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

	if _, err := provider.UpTo(ctx, 131); err != nil {
		t.Fatalf("apply decision timestamp repair: %v", err)
	}
	for hash, want := range map[string]struct{ updated, removed time.Time }{
		"decided":   {decidedAt, decidedAt},
		"untouched": {untouchedAt, time.Time{}},
	} {
		clip, err := s.GetClip(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		pipeline, _, err := s.GetClipPipeline(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		if !clip.UpdatedAt.Equal(want.updated) || !clip.RemovedAt.Equal(want.removed) || !pipeline.UpdatedAt.Equal(want.updated) {
			t.Fatalf("%s after repair: clip updated %v removed %v, pipeline updated %v; want %v, %v",
				hash, clip.UpdatedAt, clip.RemovedAt, pipeline.UpdatedAt, want.updated, want.removed)
		}
	}
}
