package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestFillerReadyDispositionsMigrationSQLite(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestRetireFillerTaggingPathMigrationSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "retire-tagging-path.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testRetireFillerTaggingPathMigration(t, s, "migrations/sqlite")
}

func testRetireFillerTaggingPathMigration(t *testing.T, s *sqlStore, migrationDir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(s.db, s.dialect, migrationDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 114); err != nil {
		t.Fatalf("migrate through 114: %v", err)
	}

	at := time.Date(2026, time.September, 18, 18, 0, 0, 0, time.UTC)
	hash := strings.Repeat("8", 64)
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO clips
		(hash, path, name, kind, duration_ms, held, source, updated_at, created_at)
		VALUES (?, ?, 'Interrupted text classification', 'commercial', 30000, ?, 'archive:classic', ?, ?)`),
		hash, hash+".mp4", true, at.Unix(), at.Unix()); err != nil {
		t.Fatal(err)
	}
	legacyTagStage := filler.StageID("tag") // retired-ok: migration fixture
	if err := insertLegacyClipPipeline(ctx, s, filler.ClipPipeline{
		ClipHash: hash, Stage: legacyTagStage, Status: filler.StatusRunning,
		Disposition: filler.DispositionRunning, Attempts: 2, ForceRun: true,
		Stages: []filler.StageRecord{
			{Stage: filler.StageProbe, Status: filler.StatusDone, At: at},
			{Stage: legacyTagStage, Status: filler.StatusDone, At: at},
		},
		EnrolledAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO settings (key, value, updated_at, updated_by, env_override)
		VALUES (?, 'true', ?, 'fixture', ?)`), "filler.ai_tagging", at.Unix(), false); err != nil { // retired-ok: migration fixture
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 115); err != nil {
		t.Fatalf("apply tagging-path retirement: %v", err)
	}
	var stage, status, stagesJSON string
	var attempts int
	var forceRun bool
	if err := s.db.QueryRowContext(ctx, s.ph(`SELECT stage, status, attempts, force_run, stages_json
		FROM filler_clip_pipeline WHERE clip_hash = ?`), hash).Scan(&stage, &status, &attempts, &forceRun, &stagesJSON); err != nil {
		t.Fatal(err)
	}
	if filler.StageID(stage) != filler.StageVision || filler.StageStatus(status) != filler.StatusQueued || attempts != 0 || forceRun {
		t.Fatalf("interrupted row = stage=%s status=%s attempts=%d force=%t", stage, status, attempts, forceRun)
	}
	var stages []filler.StageRecord
	if err := json.Unmarshal([]byte(stagesJSON), &stages); err != nil {
		t.Fatal(err)
	}
	if len(stages) != 1 || stages[0].Stage != filler.StageProbe {
		t.Fatalf("legacy text-classification record survived: %+v", stages)
	}
	var settings int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key = 'filler.ai_tagging'`).Scan(&settings); err != nil { // retired-ok: migration assertion
		t.Fatal(err)
	}
	if settings != 0 {
		t.Fatalf("retired setting rows = %d", settings)
	}
}

// insertLegacyClipPipeline deliberately writes the pre-00113 shape used by migration fixtures.
// The production writer always speaks the current schema and must not grow a compatibility path.
func insertLegacyClipPipeline(ctx context.Context, st Store, p filler.ClipPipeline) error {
	s, ok := adapterOf(st)
	if !ok {
		return fmt.Errorf("%T is not the SQL store", st)
	}
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

// readClipPipelineRow reads the pipeline columns the migration tests assert on. The pipeline's own
// methods are the filler store's (internal/fillerstore), which a core test cannot import, so the
// tests seed with insertLegacyClipPipeline and read back here.
func readClipPipelineRow(ctx context.Context, st Store, hash string) (filler.ClipPipeline, bool, error) {
	s, ok := adapterOf(st)
	if !ok {
		return filler.ClipPipeline{}, false, fmt.Errorf("%T is not the SQL store", st)
	}
	var p filler.ClipPipeline
	var stage, status, disposition string
	var nextRun, updatedAt int64
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT clip_hash, stage, status, disposition, attempts, next_run, updated_at
		FROM filler_clip_pipeline WHERE clip_hash = ?`), hash).
		Scan(&p.ClipHash, &stage, &status, &disposition, &p.Attempts, &nextRun, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return filler.ClipPipeline{}, false, nil
	}
	if err != nil {
		return filler.ClipPipeline{}, false, err
	}
	p.Stage, p.Status, p.Disposition = filler.StageID(stage), filler.StageStatus(status), filler.Disposition(disposition)
	p.NextRun, p.UpdatedAt = fromEpoch(nextRun), fromEpoch(updatedAt)
	return p, true, nil
}
