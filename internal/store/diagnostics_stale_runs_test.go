package store

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// Orphaned `running` rows (#1401) were never finalized across container restarts, and because
// retention refuses to delete a running row they also could never be purged. The store owns the
// reconciliation so it runs as one bounded statement against the real schema.
func TestFinalizeStaleDiagnosticProcessRuns(t *testing.T) {
	ctx := context.Background()
	st := openRetentionStore(t)
	started := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	seed := func(id, instance string, status diagnostics.ProcessStatus, ended int64) {
		t.Helper()
		err := st.UpsertDiagnosticProcessRun(ctx, diagnostics.ProcessRun{
			ID: id, Purpose: "playout_parent", InstanceID: instance, ChannelID: "ch_1",
			StartedAt: started, EndedAt: ended, Status: status, UpdatedAt: started,
		})
		if err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("stale-1", "inst-a", diagnostics.ProcessRunning, 0)
	seed("stale-2", "inst-a", diagnostics.ProcessRunning, 0)
	seed("other-instance", "inst-b", diagnostics.ProcessRunning, 0)
	seed("done", "inst-a", diagnostics.ProcessSucceeded, started+5000)

	now := time.Date(2026, 9, 25, 8, 0, 0, 0, time.UTC)
	n, err := st.FinalizeStaleDiagnosticProcessRuns(ctx, "inst-a", now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("finalized %d rows, want 2", n)
	}
	for _, id := range []string{"stale-1", "stale-2"} {
		run, err := st.GetDiagnosticProcessRun(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != diagnostics.ProcessInterrupted || run.EndedAt != now.UnixMilli() || run.UpdatedAt != now.UnixMilli() {
			t.Errorf("%s = status %q ended %d updated %d, want interrupted at %d",
				id, run.Status, run.EndedAt, run.UpdatedAt, now.UnixMilli())
		}
		if run.TerminationReason == "" {
			t.Errorf("%s has no termination reason", id)
		}
	}
	// Another instance's live run and an already-finished run are untouched.
	if run, _ := st.GetDiagnosticProcessRun(ctx, "other-instance"); run.Status != diagnostics.ProcessRunning || run.EndedAt != 0 {
		t.Errorf("other instance's run was touched: %+v", run)
	}
	if run, _ := st.GetDiagnosticProcessRun(ctx, "done"); run.Status != diagnostics.ProcessSucceeded || run.EndedAt != started+5000 {
		t.Errorf("finished run was touched: %+v", run)
	}
	// Once finalized the row is retention-deletable, which a running row never is.
	if deleted, err := st.DeleteDiagnosticProcessRun(ctx, "stale-1"); err != nil || !deleted {
		t.Errorf("finalized row not deletable: deleted=%v err=%v", deleted, err)
	}
	// Idempotent.
	if n, err := st.FinalizeStaleDiagnosticProcessRuns(ctx, "inst-a", now); err != nil || n != 0 {
		t.Errorf("second pass = %d, %v; want 0, nil", n, err)
	}
}
