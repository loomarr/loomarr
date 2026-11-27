package filler_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// readyRowAtFinish seeds a clip whose ladder is complete up to the finish rung, so the next pass
// goes straight to the terminal commit.
func readyRowAtFinish(st *pipeMemStore, hash string) {
	seedEnrolled(st, hash)
	row := st.rows[hash]
	for _, id := range pipelinestate.StageOrder {
		row.Stages = append(row.Stages, pipelinestate.StageRecord{Stage: id, Status: pipelinestate.StatusDone})
	}
	row.Stage, row.Status = pipelinestate.StageFinish, pipelinestate.StatusQueued
	st.rows[hash] = row
}

// A clip whose terminal commit keeps being refused as stale can never publish. It gets the
// ordinary attempts, then rests 30 days instead of taking a work slot every two hours for good
// (#3855, decision 12 of #3838). Nothing is rejected, so the clip is never let go for it.
func TestPipeline_StaleReadyCommitRestsAfterItsAttempts(t *testing.T) {
	st := newPipeMemStore()
	readyRowAtFinish(st, "90001001")
	st.commitErr = fmt.Errorf("%w: clip and pipeline identity must be current", filler.ErrReadyStale)

	now := time.Unix(1_800_000_000, 0).UTC()
	p := filler.NewPipeline(st, st, asSlice(allStages()), filler.DefaultBudget(), nil, func() time.Time { return now }, nil)

	var lastAttemptAt time.Time
	for attempt := 1; attempt <= filler.MaxAttempts; attempt++ {
		lastAttemptAt = now
		if _, err := p.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		row := st.rows["90001001"]
		if row.Disposition != pipelinestate.DispositionRunning {
			t.Fatalf("attempt %d: disposition = %q, want running (a stale commit rejects nothing)", attempt, row.Disposition)
		}
		if attempt < filler.MaxAttempts && row.NextRun.After(now.Add(3*time.Hour)) {
			t.Fatalf("attempt %d: next run %v is already resting; the clip is owed %d attempts", attempt, row.NextRun, filler.MaxAttempts)
		}
		now = row.NextRun.Add(time.Second)
	}

	row := st.rows["90001001"]
	if rest := row.NextRun.Sub(lastAttemptAt); rest < pipelinestate.FailureRest {
		t.Fatalf("after %d stale commits the clip rests %v, want at least %v", filler.MaxAttempts, rest, pipelinestate.FailureRest)
	}

	// A day on, the clip is not due: the pass does no work on it.
	now = lastAttemptAt.Add(24 * time.Hour)
	res, err := p.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 0 || res.Advanced != 0 {
		t.Fatalf("a resting clip took work: %+v", res)
	}

	// When the rest ends the clip is tried again, so a cause that cleared lets it publish.
	st.commitErr = nil
	now = lastAttemptAt.Add(pipelinestate.FailureRest + time.Minute)
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.rows["90001001"].Disposition; got != pipelinestate.DispositionReady {
		t.Fatalf("after the rest the clip is %q, want ready", got)
	}
}

// A refused commit that is not a stale clip (the catalog could not be written) keeps its short
// backoff: the cause is the install's, and it clears on its own.
func TestPipeline_TransientReadyCommitFailureNeverRestsLong(t *testing.T) {
	st := newPipeMemStore()
	readyRowAtFinish(st, "90001002")
	st.commitErr = errors.New("catalog write failed")

	now := time.Unix(1_800_000_000, 0).UTC()
	p := filler.NewPipeline(st, st, asSlice(allStages()), filler.DefaultBudget(), nil, func() time.Time { return now }, nil)
	for range 2 * filler.MaxAttempts {
		if _, err := p.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		row := st.rows["90001002"]
		if row.NextRun.After(now.Add(3 * time.Hour)) {
			t.Fatalf("a transient commit failure rests %v; only a stale clip rests long", row.NextRun.Sub(now))
		}
		now = row.NextRun.Add(time.Second)
	}
}

// An AI backend that is down or out of budget says nothing about the clip: the rung waits without
// spending an attempt, so the clip is neither skipped past it nor failed (#3855, decision 12).
func TestPipeline_UnavailableAIWaitsWithoutSpendingAttempts(t *testing.T) {
	st := newPipeMemStore()
	seedEnrolled(st, "90001003")
	stages := allStages()
	stages[pipelinestate.StageVision].err = fmt.Errorf("vision model for a/b/90001003.mp4: %w", filler.ErrAIUnavailable)

	now := time.Unix(1_800_000_000, 0).UTC()
	p := filler.NewPipeline(st, st, asSlice(stages), filler.DefaultBudget(), nil, func() time.Time { return now }, nil)

	for pass := 0; pass < 3*filler.MaxAttempts; pass++ {
		res, err := p.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if res.Failed != 0 {
			t.Fatalf("pass %d counted the unavailable AI as a failure: %+v", pass, res)
		}
		row := st.rows["90001003"]
		if row.Disposition != pipelinestate.DispositionRunning || row.Stage != pipelinestate.StageVision {
			t.Fatalf("pass %d: row = %s/%s, want it waiting at vision", pass, row.Disposition, row.Stage)
		}
		if row.Attempts != 0 || row.Status == pipelinestate.StatusFailed {
			t.Fatalf("pass %d spent an attempt on the unavailable AI: attempts=%d status=%s", pass, row.Attempts, row.Status)
		}
		if !row.NextRun.After(now) {
			t.Fatalf("pass %d: the rung is due again at once (next run %v); it must wait", pass, row.NextRun)
		}
		now = row.NextRun.Add(time.Second)
	}

	// When the AI is back the clip carries on from the rung it waited at.
	stages[pipelinestate.StageVision].err = nil
	if _, err := p.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.rows["90001003"].Disposition; got != pipelinestate.DispositionReady {
		t.Fatalf("after the AI returned the clip is %q, want ready", got)
	}
}
