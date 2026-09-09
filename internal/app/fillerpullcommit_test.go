package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

func TestIngestPull_RequiresApprovalCommit(t *testing.T) {
	a := fillerServiceAdapter{fetcher: successfulClipIngestor{}}
	if _, err := a.IngestPull(t.Context(), "pull", []filler.AcquisitionTarget{{Kind: "archive", URL: "https://archive.org/details/one"}}, nil); err == nil {
		t.Fatal("pull ingest accepted no approval commit")
	}
}

func TestIngestPull_CommitFailureNeverLaunches(t *testing.T) {
	commitErr := errors.New("approval transaction failed")
	a := fillerServiceAdapter{
		fetcher: successfulClipIngestor{}, newID: func() string { return "uncommitted" },
		start: func(time.Duration, func(context.Context) error, func(context.Context, error)) error {
			t.Error("failed commit reached the downloader launcher")
			return nil
		},
	}
	_, err := a.IngestPull(t.Context(), "pull", []filler.AcquisitionTarget{{SourceID: "classic", Kind: "archive", URL: "https://archive.org/details/one"}},
		func(_ context.Context, run filler.AcquisitionRun) error {
			if run.Status != filler.AcquisitionQueued || run.ID != "uncommitted" || run.PullID != "pull" {
				t.Errorf("commit received wrong run: %+v", run)
			}
			return commitErr
		})
	if !errors.Is(err, commitErr) {
		t.Fatalf("ingest = %v, want commit failure", err)
	}
}

func TestIngestPull_PostCommitInterruption(t *testing.T) {
	for _, crash := range []bool{false, true} {
		name := "LaunchFailure"
		if crash {
			name = "CrashBeforeLaunch"
		}
		t.Run(name, func(t *testing.T) {
			dsn := "sqlite://" + filepath.Join(t.TempDir(), "pull.db")
			st, err := store.Open(t.Context(), dsn, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
			p := filler.Pull{ID: "pull", Status: filler.PullPending, CreatedAt: now,
				Plan: []filler.PullPlanRow{{SourceID: "classic", Name: "Reviewed item"}}}
			if err := st.UpsertPull(t.Context(), p); err != nil {
				t.Fatal(err)
			}
			p.Status, p.DecidedAt, p.DecidedBy, p.Note = filler.PullApproved, now, "admin", "keep this exact item"
			fetched := make(chan []clipfetch.Source, 1)
			interruption := errors.New("application stopped before launch")
			a := fillerServiceAdapter{
				fetcher: recordingClipIngestor{sources: fetched}, acquisitions: st,
				newID: func() string { return "only-run" }, now: func() time.Time { return now },
				start: func(time.Duration, func(context.Context) error, func(context.Context, error)) error {
					if crash {
						// Abruptly leave the post-commit/pre-launch seam, bypassing completion.
						panic(interruption)
					}
					return interruption
				},
			}
			targets := []filler.AcquisitionTarget{{SourceID: "classic", Kind: "archive", URL: "https://archive.org/details/one"}}
			commit := func(ctx context.Context, run filler.AcquisitionRun) error { return st.CommitPullApproval(ctx, p, run) }
			func() {
				if crash {
					defer func() {
						if got := recover(); got != interruption {
							t.Errorf("crash seam = %v, want interruption", got)
						}
					}()
				}
				if _, err := a.IngestPull(t.Context(), p.ID, targets, commit); !errors.Is(err, interruption) {
					t.Errorf("launch = %v, want interruption", err)
				}
			}()
			run, err := st.GetAcquisitionRun(t.Context(), "only-run", now)
			wantStatus := filler.AcquisitionError
			if crash {
				wantStatus = filler.AcquisitionQueued
			}
			if err != nil || run.Status != wantStatus {
				t.Fatalf("interrupted run = %+v (%v), want %s", run, err, wantStatus)
			}
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}
			st, err = store.Open(t.Context(), dsn, true)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := st.RecoverInterruptedAcquisitionRuns(t.Context(), now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			a.acquisitions = st
			if _, err := a.IngestPull(t.Context(), p.ID, targets, commit); !errors.Is(err, store.ErrPullNotPending) {
				t.Fatalf("retry after reopening = %v, want conflict before launch", err)
			}
			decision, err := st.GetPull(t.Context(), p.ID)
			if err != nil || decision.Status != filler.PullApproved || decision.Note != p.Note || decision.DecidedBy != p.DecidedBy || !decision.DecidedAt.Equal(p.DecidedAt) {
				t.Fatalf("decision audit changed after restart: %+v (%v)", decision, err)
			}
			runs, err := st.ListAcquisitionRuns(t.Context(), 10, now.Add(time.Minute))
			if err != nil || len(runs) != 1 || runs[0].ID != "only-run" || runs[0].Status != filler.AcquisitionError || runs[0].Error == "" {
				t.Fatalf("restart runs = %+v (%v), want one visible error", runs, err)
			}
			select {
			case got := <-fetched:
				t.Fatalf("interrupted/retried approval downloaded: %+v", got)
			default:
			}
		})
	}
}
