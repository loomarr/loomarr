package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
)

// The 00050 data migration: compilations parked at `split` go back on the belt (§10 V54).
//
// ⚠ **This runs the SHIPPED migration file, not a copy of its SQL.** A test that restated the
// UPDATE would keep passing after someone widened the migration's WHERE clause, which is the only
// part of a six-line data migration that can actually be wrong. Reading the embedded FS means the
// artifact under test is the artifact that ships.
//
// ⚠ It cannot exercise goose's ordering (there is no migrate-to-version-N helper in this tree, and
// building one for a single data migration is more machinery than the risk warrants). What it
// pins is the PREDICATE: which rows move, and — far more important — which rows do not.
func TestUnparkSplitReels_MovesOnlyReelsTheGateCouldNotPass(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "unpark.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	now := time.Now().UTC().Truncate(time.Second)
	rows := []filler.ClipPipeline{
		// THE case: a reel refused by a gate that structurally could not pass.
		{ClipHash: "h:parked-reel", Stage: filler.StageSplit, Status: filler.StatusDone,
			Disposition: filler.DispositionReview, Attempts: 3, NextRun: now.Add(time.Hour), UpdatedAt: now},
		// Mid-detection. Already claimable; the migration must not reset its attempts or clock.
		{ClipHash: "h:detecting", Stage: filler.StageSplit, Status: filler.StatusQueued,
			Disposition: filler.DispositionRunning, Attempts: 1, NextRun: now.Add(time.Hour), UpdatedAt: now},
		// ⚠ A GENUINE operator decision, at a different rung. `score`+`review` means a human was
		// asked to judge a clip and has not answered. Un-parking it would silently retract the
		// question — the failure mode that makes a repair migration dangerous.
		{ClipHash: "h:awaiting-human", Stage: filler.StageScore, Status: filler.StatusDone,
			Disposition: filler.DispositionReview, Attempts: 2, NextRun: now.Add(time.Hour), UpdatedAt: now},
		// A rejected reel stays rejected: `rejected` is a fact about the file, not a lost gate.
		{ClipHash: "h:rejected-reel", Stage: filler.StageSplit, Status: filler.StatusDone,
			Disposition: filler.DispositionRejected, Attempts: 1, NextRun: now.Add(time.Hour), UpdatedAt: now},
	}
	for _, r := range rows {
		if err := s.UpsertClipPipeline(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	// Re-run the shipped migration body against rows that now look pre-migration.
	raw, err := migrationFS.ReadFile("migrations/sqlite/00050_unpark_split_reels.sql")
	if err != nil {
		t.Fatal(err)
	}
	stmt := upSection(t, string(raw))
	if !strings.Contains(stmt, "filler_clip_pipeline") {
		t.Fatalf("extracted the wrong section of the migration: %q", stmt)
	}
	if _, err := PoolOf(s).ExecContext(ctx, stmt); err != nil {
		t.Fatal(err)
	}

	get := func(hash string) filler.ClipPipeline {
		t.Helper()
		row, ok, err := s.GetClipPipeline(ctx, hash)
		if err != nil || !ok {
			t.Fatalf("get %s: (%v, %v)", hash, ok, err)
		}
		return row
	}

	parked := get("h:parked-reel")
	if parked.Disposition != filler.DispositionRunning || parked.Status != filler.StatusQueued {
		t.Errorf("parked reel = %s/%s, want running/queued — it is unreachable otherwise",
			parked.Disposition, parked.Status)
	}
	if parked.Attempts != 0 {
		t.Errorf("parked reel kept %d attempts; retries spent losing to an unwinnable gate must not "+
			"count against the passes it now needs", parked.Attempts)
	}
	if !parked.NextRun.IsZero() {
		t.Errorf("parked reel next_run = %v, want zero (due now)", parked.NextRun)
	}

	// ⚠ The assertions that matter more than the one above.
	if h := get("h:awaiting-human"); h.Disposition != filler.DispositionReview {
		t.Errorf("a clip awaiting a HUMAN at %s was un-parked (%s) — the migration retracted a "+
			"question an operator was asked", h.Stage, h.Disposition)
	}
	if r := get("h:rejected-reel"); r.Disposition != filler.DispositionRejected {
		t.Errorf("a rejected reel was revived: %s", r.Disposition)
	}
	if d := get("h:detecting"); d.Attempts != 1 || d.NextRun.IsZero() {
		t.Errorf("a reel mid-detection was disturbed: attempts=%d nextRun=%v", d.Attempts, d.NextRun)
	}
}

// upSection returns the statements between `-- +goose Up` and `-- +goose Down`, with comment lines
// stripped so the result is executable.
func upSection(t *testing.T, sql string) string {
	t.Helper()
	_, after, ok := strings.Cut(sql, "-- +goose Up")
	if !ok {
		t.Fatal("migration has no `-- +goose Up` marker")
	}
	body, _, ok := strings.Cut(after, "-- +goose Down")
	if !ok {
		t.Fatal("migration has no `-- +goose Down` marker")
	}
	var keep []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}
