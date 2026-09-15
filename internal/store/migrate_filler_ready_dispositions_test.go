package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestFillerReadyDispositionsMigrationSQLite(t *testing.T) {
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "ready-dispositions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testFillerReadyDispositionsMigration(t, s, "migrations/sqlite")
}

func testFillerReadyDispositionsMigration(t *testing.T, s *sqlStore, migrationDir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(s.db, s.dialect, migrationDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 107); err != nil {
		t.Fatalf("migrate through 107: %v", err)
	}

	at := time.Date(2026, time.September, 13, 20, 0, 0, 0, time.UTC)
	seed := func(hash string, composite bool) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO clips
			(hash, path, name, kind, duration_ms, held, is_composite, source, updated_at)
			VALUES (?, ?, ?, 'unclassified', 30000, ?, ?, 'archive:classic', ?)`),
			hash, hash+".mp4", hash, !composite, composite, at.Unix()); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertClipPipeline(ctx, filler.ClipPipeline{
			ClipHash: hash, Stage: filler.StageScore, Status: filler.StatusDone, Progress: 100,
			Disposition: filler.Disposition("filed"), EnrolledAt: at, UpdatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	seed("playable", false)
	seed("container", true)

	if _, err := provider.UpTo(ctx, 108); err != nil {
		t.Fatalf("apply ready dispositions migration: %v", err)
	}
	for hash, want := range map[string]filler.Disposition{
		"playable": filler.DispositionReady, "container": filler.DispositionComplete,
	} {
		row, found, err := s.GetClipPipeline(ctx, hash)
		if err != nil || !found || row.Disposition != want {
			t.Fatalf("%s pipeline = %+v, found=%v err=%v; want %q", hash, row, found, err, want)
		}
	}
}

func TestRemoveFillerAdmissionRungMigrationSQLite(t *testing.T) {
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "remove-admission-rung.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testRemoveFillerAdmissionRungMigration(t, s, "migrations/sqlite")
}

func testRemoveFillerAdmissionRungMigration(t *testing.T, s *sqlStore, migrationDir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(s.db, s.dialect, migrationDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 108); err != nil {
		t.Fatalf("migrate through 108: %v", err)
	}

	at := time.Date(2026, time.September, 13, 21, 0, 0, 0, time.UTC)
	hash := strings.Repeat("9", 64)
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO clips
		(hash, path, name, kind, duration_ms, held, source, updated_at)
		VALUES (?, ?, 'Interrupted legacy clip', 'commercial', 30000, ?, 'archive:classic', ?)`),
		hash, hash+".mp4", true, at.Unix()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClipPipeline(ctx, filler.ClipPipeline{
		ClipHash: hash, Stage: filler.StageID("admission"), Status: filler.StatusRunning,
		Disposition: filler.DispositionRunning, Attempts: 2, ForceRun: true,
		Stages: []filler.StageRecord{
			{Stage: filler.StageProbe, Status: filler.StatusDone, At: at},
			{Stage: filler.StageID("admission"), Status: filler.StatusDone, At: at},
		},
		EnrolledAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 109); err != nil {
		t.Fatalf("apply admission-rung removal: %v", err)
	}
	row, found, err := s.GetClipPipeline(ctx, hash)
	if err != nil || !found {
		t.Fatalf("pipeline = %+v, found=%t err=%v", row, found, err)
	}
	if row.Stage != filler.StageScore || row.Status != filler.StatusQueued || row.Attempts != 0 ||
		row.ForceRun || row.Disposition != filler.DispositionRunning {
		t.Fatalf("interrupted legacy row = %+v", row)
	}
	if len(row.Stages) != 1 || row.Stages[0].Stage != filler.StageProbe {
		t.Fatalf("legacy admission record survived: %+v", row.Stages)
	}
}
