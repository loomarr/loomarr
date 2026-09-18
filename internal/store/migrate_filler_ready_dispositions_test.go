package store

import (
	"context"
	"encoding/json"
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
		if err := insertLegacyClipPipeline(ctx, s, filler.ClipPipeline{
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
		var got string
		if err := s.db.QueryRowContext(ctx, s.ph(`SELECT disposition FROM filler_clip_pipeline WHERE clip_hash = ?`), hash).Scan(&got); err != nil || filler.Disposition(got) != want {
			t.Fatalf("%s disposition = %q, err=%v; want %q", hash, got, err, want)
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
	if err := insertLegacyClipPipeline(ctx, s, filler.ClipPipeline{
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
	var stage, status, disposition, stagesJSON string
	var attempts int
	var forceRun bool
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT stage, status, disposition, attempts, force_run, stages_json
		FROM filler_clip_pipeline WHERE clip_hash = ?`), hash).Scan(
		&stage, &status, &disposition, &attempts, &forceRun, &stagesJSON); err != nil {
		t.Fatal(err)
	}
	if filler.StageID(stage) != filler.StageScore || filler.StageStatus(status) != filler.StatusQueued || attempts != 0 ||
		forceRun || filler.Disposition(disposition) != filler.DispositionRunning {
		t.Fatalf("interrupted legacy row = stage=%s status=%s disposition=%s attempts=%d force=%t", stage, status, disposition, attempts, forceRun)
	}
	var stages []filler.StageRecord
	if err := json.Unmarshal([]byte(stagesJSON), &stages); err != nil {
		t.Fatal(err)
	}
	if len(stages) != 1 || stages[0].Stage != filler.StageProbe {
		t.Fatalf("legacy admission record survived: %+v", stages)
	}
}

// insertLegacyClipPipeline deliberately writes the pre-00113 shape used by migration fixtures.
// The production writer always speaks the current schema and must not grow a compatibility path.
func insertLegacyClipPipeline(ctx context.Context, s *sqlStore, p filler.ClipPipeline) error {
	raw := "[]"
	if len(p.Stages) > 0 {
		encoded, err := json.Marshal(p.Stages)
		if err != nil {
			return err
		}
		raw = string(encoded)
	}
	_, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO filler_clip_pipeline
		(clip_hash, acquisition_id, stage, status, progress, disposition, reject_reason,
		 reject_detail, attempts, force_run, next_run, stages_json, enrolled_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ClipHash, p.AcquisitionID, string(p.Stage), string(p.Status), p.Progress,
		string(p.Disposition), string(p.RejectReason), p.RejectDetail, p.Attempts, p.ForceRun,
		epoch(p.NextRun), raw, epoch(p.EnrolledAt), epoch(p.UpdatedAt))
	return err
}
