package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestQuarantineLegacyFillerMigration_SQLite(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "quarantine.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testQuarantineLegacyFillerMigration(t, s, PoolOf(s), "migrations/sqlite/00098_quarantine_legacy_filler.sql")
}

// testQuarantineLegacyFillerMigration runs the shipped migration body over a deliberately old
// catalog state. Running copied SQL would prove the test's transcription rather than the artifact
// that upgrades an operator's database.
func testQuarantineLegacyFillerMigration(t *testing.T, s Store, db *sql.DB, migrationPath string) {
	t.Helper()
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()
	seed := func(hash string, held, composite bool, removed time.Time, disposition filler.Disposition) {
		t.Helper()
		if err := s.UpsertClip(ctx, Clip{Clip: filler.Clip{
			Hash: hash, Path: hash + ".mp4", Name: hash, Kind: filler.Commercial,
			DurationMs: 30_000, Held: held, IsComposite: composite, RemovedAt: removed,
		}, UpdatedAt: at}); err != nil {
			t.Fatal(err)
		}
		if disposition != "" {
			if err := s.UpsertClipPipeline(ctx, filler.ClipPipeline{
				ClipHash: hash, Stage: filler.StageScore, Status: filler.StatusDone,
				Progress: 100, Disposition: disposition, EnrolledAt: at, UpdatedAt: at,
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	seed("legacy-playable", false, false, time.Time{}, filler.DispositionFiled)
	seed("legacy-without-pipeline", false, false, time.Time{}, "")
	seed("already-held", true, false, time.Time{}, filler.DispositionReview)
	seed("composite", false, true, time.Time{}, filler.DispositionFiled)
	seed("removed", false, false, at, filler.DispositionFiled)

	// Simulate the retired publisher's physical marker. The migration clears it everywhere even
	// though the application no longer reads the column.
	if _, err := db.ExecContext(ctx, `UPDATE clips SET auto_filed = TRUE`); err != nil {
		t.Fatal(err)
	}
	raw, err := migrationFS.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, upSection(t, string(raw))); err != nil {
		t.Fatal(err)
	}

	for _, hash := range []string{"legacy-playable", "legacy-without-pipeline", "already-held"} {
		clip, err := s.GetClip(ctx, hash)
		if err != nil || !clip.Held {
			t.Errorf("%s = held:%v err:%v, want quarantined", hash, clip.Held, err)
		}
	}
	for _, hash := range []string{"composite", "removed"} {
		clip, err := s.GetClip(ctx, hash)
		if err != nil || clip.Held {
			t.Errorf("%s = held:%v err:%v, want lifecycle unchanged", hash, clip.Held, err)
		}
	}
	row, found, err := s.GetClipPipeline(ctx, "legacy-playable")
	if err != nil || !found || row.Disposition != filler.DispositionReview || row.Status != filler.StatusDone {
		t.Errorf("legacy playable pipeline = %+v found:%v err:%v, want done/review", row, found, err)
	}
	for _, hash := range []string{"already-held", "composite", "removed"} {
		row, found, err := s.GetClipPipeline(ctx, hash)
		want := filler.DispositionReview
		if hash != "already-held" {
			want = filler.DispositionFiled
		}
		if err != nil || !found || row.Disposition != want {
			t.Errorf("%s pipeline = %+v found:%v err:%v, want %s", hash, row, found, err, want)
		}
	}
	var marked int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clips WHERE auto_filed = TRUE`).Scan(&marked); err != nil {
		t.Fatal(err)
	}
	if marked != 0 {
		t.Errorf("legacy auto_filed markers = %d, want 0", marked)
	}
}
