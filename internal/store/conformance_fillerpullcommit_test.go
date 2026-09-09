package store

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func testFillerPullCommit(t *testing.T, newStore NewStoreFunc) {
	fixture := func(t *testing.T) (Store, filler.Pull, filler.AcquisitionRun) {
		t.Helper()
		s := newStore(t)
		now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
		p := filler.Pull{ID: "atomic-pull", Title: "Reviewed clips", Status: filler.PullPending, CreatedAt: now,
			Plan: []filler.PullPlanRow{{SourceID: "classic", Name: "Keep"}, {SourceID: "psa", Name: "Drop"}}}
		if err := s.UpsertPull(t.Context(), p); err != nil {
			t.Fatal(err)
		}
		p.Status, p.DecidedAt, p.DecidedBy, p.Note = filler.PullApproved, now.Add(time.Minute), "admin-winner", "exact note"
		p.Plan[1].Dropped = true
		run := filler.AcquisitionRun{ID: "atomic-run", PullID: p.ID, Trigger: filler.AcquisitionPull,
			Status: filler.AcquisitionQueued, Requested: 1, StartedAt: p.DecidedAt, UpdatedAt: p.DecidedAt}
		return s, p, run
	}
	t.Run("ConcurrentApprovalsAndRestart", func(t *testing.T) {
		s, p, run := fixture(t)
		type decision struct {
			pull filler.Pull
			run  filler.AcquisitionRun
			err  error
		}
		ready, release, results := make(chan struct{}, 2), make(chan struct{}), make(chan decision, 2)
		for _, id := range []string{"one", "two"} {
			go func() {
				candidate, execution := p, run
				candidate.Note, candidate.DecidedBy = id, "admin-"+id
				execution.ID = id
				ready <- struct{}{}
				<-release
				results <- decision{candidate, execution, s.CommitPullApproval(t.Context(), candidate, execution)}
			}()
		}
		<-ready
		<-ready
		close(release)
		var winner decision
		wins, conflicts := 0, 0
		for range 2 {
			result := <-results
			if result.err == nil {
				wins++
				winner = result
			} else if errors.Is(result.err, ErrPullNotPending) {
				conflicts++
			} else {
				t.Fatal(result.err)
			}
		}
		if wins != 1 || conflicts != 1 {
			t.Fatalf("wins/conflicts = %d/%d, want 1/1", wins, conflicts)
		}
		got, err := s.GetPull(t.Context(), p.ID)
		if err != nil || !reflect.DeepEqual(got.Plan, winner.pull.Plan) || got.Note != winner.pull.Note ||
			got.DecidedBy != winner.pull.DecidedBy || !got.DecidedAt.Equal(winner.pull.DecidedAt) || got.Status != filler.PullApproved {
			t.Fatalf("winning audit = %+v (%v), want %+v", got, err, winner.pull)
		}
		// Simulate startup after the transaction committed but before any downloader launched.
		n, err := s.RecoverInterruptedAcquisitionRuns(t.Context(), p.DecidedAt.Add(time.Minute))
		if err != nil || n != 1 {
			t.Fatalf("recovered = %d (%v), want one", n, err)
		}
		runs, err := s.ListAcquisitionRuns(t.Context(), 10, p.DecidedAt.Add(time.Minute))
		if err != nil || len(runs) != 1 || runs[0].ID != winner.run.ID || runs[0].Status != filler.AcquisitionError || runs[0].Error == "" {
			t.Fatalf("recovered runs = %+v (%v), want one visible interrupted error", runs, err)
		}
		if err := s.CommitPullApproval(t.Context(), p, run); !errors.Is(err, ErrPullNotPending) {
			t.Fatalf("restart retry = %v, want conflict", err)
		}
		if n, err := s.RecoverInterruptedAcquisitionRuns(t.Context(), p.DecidedAt.Add(2*time.Minute)); err != nil || n != 0 {
			t.Fatalf("second recovery = %d (%v), want no repeated work", n, err)
		}
	})

	t.Run("SnapshotWriterCannotBypassApproval", func(t *testing.T) {
		s, p, run := fixture(t)
		if err := s.UpsertAcquisitionRun(t.Context(), run); err == nil {
			t.Fatal("snapshot writer created unapproved pull work")
		}
		if err := s.CommitPullApproval(t.Context(), p, run); err != nil {
			t.Fatal(err)
		}
		another := run
		another.ID = "another-run"
		if err := s.UpsertAcquisitionRun(t.Context(), another); err == nil {
			t.Fatal("snapshot writer created second pull run")
		}
		run.PullID = ""
		run.Trigger = filler.AcquisitionManual
		if err := s.UpsertAcquisitionRun(t.Context(), run); err == nil {
			t.Fatal("snapshot writer erased pull ownership")
		}
	})
	t.Run("RunInsertFailureRollsBackDecision", func(t *testing.T) {
		s, p, run := fixture(t)
		occupied := run
		occupied.PullID, occupied.Trigger = "", filler.AcquisitionManual
		if err := s.UpsertAcquisitionRun(t.Context(), occupied); err != nil {
			t.Fatal(err)
		}
		if err := s.CommitPullApproval(t.Context(), p, run); err == nil {
			t.Fatal("duplicate run identity committed a decision")
		}
		got, err := s.GetPull(t.Context(), p.ID)
		if err != nil || got.Status != filler.PullPending || got.Note != "" || !got.DecidedAt.IsZero() || got.Plan[1].Dropped {
			t.Fatalf("failed commit changed audit: %+v (%v)", got, err)
		}
		run.ID = "fresh-run"
		if err := s.CommitPullApproval(t.Context(), p, run); err != nil {
			t.Fatalf("retry after rollback: %v", err)
		}
	})
	t.Run("HistoricalRunsRemainAuditAndPreventAnother", func(t *testing.T) {
		s, p, run := fixture(t)
		for _, id := range []string{"historical-one", "historical-two"} {
			old := run
			old.ID, old.Status = id, filler.AcquisitionError
			// Imported historical snapshots are seeded directly; the live writer
			// now requires an approval commit before creating a bound run.
			impl := s.(*sqlStore)
			if _, err := impl.db.ExecContext(t.Context(), impl.ph(acquisitionRunInsert), acquisitionRunArgs(old)...); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.CommitPullApproval(t.Context(), p, run); !errors.Is(err, ErrPullNotPending) {
			t.Fatalf("historical acquisition retry = %v, want conflict", err)
		}
		runs, err := s.ListAcquisitionRuns(t.Context(), 10, p.DecidedAt)
		if err != nil || len(runs) != 2 {
			t.Fatalf("historical audit = %+v (%v), want both original runs", runs, err)
		}
		p.Status = filler.PullDismissed
		if err := s.DismissPull(t.Context(), p); err != nil {
			t.Fatalf("settle historical pending proposal: %v", err)
		}
		after, err := s.ListAcquisitionRuns(t.Context(), 10, p.DecidedAt)
		if err != nil || !reflect.DeepEqual(after, runs) {
			t.Fatalf("dismissal rewrote acquisition audit: %+v (%v)", after, err)
		}
		decision, err := s.GetPull(t.Context(), p.ID)
		if err != nil || decision.Status != filler.PullDismissed {
			t.Fatalf("historical proposal not settled: %+v (%v)", decision, err)
		}
	})
	t.Run("BindingFailureRollsBackRunAndDecision", func(t *testing.T) {
		s, p, run := fixture(t)
		// An existing inconsistent binding forces the last insert to fail. The same
		// SQL fixture exercises both backends without a dialect-specific fault hook.
		prior := run
		prior.ID, prior.PullID, prior.Trigger = "prior-binding", "", filler.AcquisitionManual
		if err := s.UpsertAcquisitionRun(t.Context(), prior); err != nil {
			t.Fatal(err)
		}
		impl := s.(*sqlStore)
		if _, err := impl.db.ExecContext(t.Context(), impl.ph(`INSERT INTO filler_pull_commits (pull_id, acquisition_id) VALUES (?, ?)`), p.ID, prior.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.CommitPullApproval(t.Context(), p, run); err == nil {
			t.Fatal("duplicate pull binding committed")
		}
		got, err := s.GetPull(t.Context(), p.ID)
		if err != nil || got.Status != filler.PullPending || got.Note != "" || got.Plan[1].Dropped {
			t.Fatalf("binding failure changed decision: %+v (%v)", got, err)
		}
		if _, err := s.GetAcquisitionRun(t.Context(), run.ID, p.DecidedAt); !errors.Is(err, ErrNotFound) {
			t.Fatalf("binding failure left a run: %v", err)
		}
		other := p
		other.ID, other.Status = "other-pull", filler.PullPending
		if err := s.UpsertPull(t.Context(), other); err != nil {
			t.Fatal(err)
		}
		if _, err := impl.db.ExecContext(t.Context(), impl.ph(`INSERT INTO filler_pull_commits (pull_id, acquisition_id) VALUES (?, ?)`), other.ID, prior.ID); err == nil {
			t.Fatal("one acquisition was bound to two pulls")
		}
	})
	for _, approveFirst := range []bool{true, false} {
		name := "DismissWins"
		if approveFirst {
			name = "ApproveWins"
		}
		t.Run(name, func(t *testing.T) {
			s, p, run := fixture(t)
			dismiss := p
			dismiss.Status, dismiss.DecidedBy = filler.PullDismissed, "other-admin"
			if approveFirst {
				if err := s.CommitPullApproval(t.Context(), p, run); err != nil {
					t.Fatal(err)
				}
				if err := s.DismissPull(t.Context(), dismiss); !errors.Is(err, ErrPullNotPending) {
					t.Fatalf("stale dismissal = %v", err)
				}
			} else {
				if err := s.DismissPull(t.Context(), dismiss); err != nil {
					t.Fatal(err)
				}
				if err := s.CommitPullApproval(t.Context(), p, run); !errors.Is(err, ErrPullNotPending) {
					t.Fatalf("stale approval = %v", err)
				}
			}
			got, err := s.GetPull(t.Context(), p.ID)
			want := dismiss
			if approveFirst {
				want = p
			}
			if err != nil || got.Status != want.Status || got.DecidedBy != want.DecidedBy {
				t.Fatalf("winner overwritten: %+v (%v)", got, err)
			}
		})
	}
}
