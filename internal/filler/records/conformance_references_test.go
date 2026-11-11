package records

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

// saveFillerSources registers an archive source for each id not yet registered, so a download run
// that names it satisfies its foreign key to filler_sources (#2793). The package's twin of
// testkit.SaveFillerSources, which it can't import.
func saveFillerSources(t *testing.T, s Store, ids ...string) {
	t.Helper()
	ctx := context.Background()
	existing, err := s.ListFillerSources(ctx)
	if err != nil {
		t.Fatalf("list filler sources: %v", err)
	}
	have := map[string]bool{}
	for _, src := range existing {
		have[src.ID] = true
	}
	for _, id := range ids {
		if id == "" || have[id] {
			continue
		}
		have[id] = true
		if err := s.UpsertFillerSource(ctx, FillerSource{
			ID: id, Kind: "archive", URI: "fixture-" + id, CreatedAt: time.Unix(1, 0), Enabled: true,
		}); err != nil {
			t.Fatalf("save filler source %s: %v", id, err)
		}
	}
}

// saveAcquisitionRun stores a finished download run with no source, so a clip pipeline row or
// ready event that names it satisfies its foreign key to filler_acquisition_runs.
func saveAcquisitionRun(t *testing.T, s Store, id string) {
	t.Helper()
	at := time.Unix(1_900_000_000, 0).UTC()
	if err := s.UpsertAcquisitionRun(context.Background(), acquisition.AcquisitionRun{
		ID: id, Trigger: acquisition.TriggerSource, Status: acquisition.RunSuccess,
		StartedAt: at, CompletedAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatalf("save acquisition run %s: %v", id, err)
	}
}

// testAcquisitionRunOutlivesItsSource pins a download whose source is deleted while it runs: the
// run's source_id is cleared (SET NULL, #2793), and the run still records its finish rather than
// being refused as a change of ownership.
func testAcquisitionRunOutlivesItsSource(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_900_000_000, 0).UTC()
	saveFillerSources(t, s, "src-deleted-mid-run")
	run := acquisition.AcquisitionRun{
		ID: "run-mid", Trigger: acquisition.TriggerSource, SourceID: "src-deleted-mid-run",
		Status: acquisition.RunRunning, StartedAt: at, UpdatedAt: at,
	}
	if err := s.UpsertAcquisitionRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFillerSource(ctx, "src-deleted-mid-run"); err != nil {
		t.Fatal(err)
	}
	run.Status, run.CompletedAt, run.UpdatedAt = acquisition.RunSuccess, at.Add(time.Minute), at.Add(time.Minute)
	if err := s.UpsertAcquisitionRun(ctx, run); err != nil {
		t.Fatalf("finish a run whose source was deleted: %v", err)
	}
	got, err := s.GetAcquisitionRun(ctx, "run-mid", at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != acquisition.RunSuccess || got.SourceID != "" {
		t.Errorf("run = status %q, source %q; want success with no source", got.Status, got.SourceID)
	}
}
