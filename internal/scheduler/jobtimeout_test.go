package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

// The per-job execution ceiling (§18.1).
//
// ⚠ **These exist because the ceiling was never ours.** `river.Config.JobTimeout` was unset and
// `riverWorker` did not implement `Timeout`, so every job on every install ran under River's
// `JobTimeoutDefault` — ONE MINUTE, a number nobody in this repo chose and nothing recorded.
//
// The reason it went unnoticed for so long is that it does not surface as a timeout.
// `exec.CommandContext` SIGKILLs its child when the deadline fires, so a job doing real media work
// reported `ffmpeg …: signal: killed` and `whisper-cli: signal: killed`, which read as a corrupt
// file or a broken binary. Measured 2026-08-11 on the maintainer's catalog: the same
// `blackdetect`/`silencedetect` pass that died as "killed" inside the job completed in **40s** run
// by hand.

func timeoutJob(name string, d time.Duration) Job {
	return Job{
		Name: name, Group: GroupSystem, Title: name, Description: "test job " + name + ".", DefaultCron: everyMinute,
		Timeout: d,
		Run:     func(context.Context) error { return nil },
	}
}

// A job's declared ceiling is the one River is told to enforce.
func TestRiverWorker_TimeoutComesFromTheJob(t *testing.T) {
	reg := NewRegistry().
		Add(timeoutJob("slow", LongJobTimeout)).
		Add(timeoutJob("quick", 0))
	s := New(newFakeStore(), reg, nil, time.Now, testLog())
	w := &riverWorker{s: s}

	if got := w.Timeout(&river.Job[jobArgs]{Args: jobArgs{Name: "slow"}}); got != LongJobTimeout {
		t.Errorf("slow job ceiling = %v, want %v — a media job left on River's default is "+
			"SIGKILLed mid-ffmpeg after one minute", got, LongJobTimeout)
	}
	// ⚠ Zero is meaningful, not missing: River reads it as "use JobTimeoutDefault", which is the
	// right answer for a sweep — one that has not finished in a minute is stuck, not slow.
	if got := w.Timeout(&river.Job[jobArgs]{Args: jobArgs{Name: "quick"}}); got != 0 {
		t.Errorf("undeclared ceiling = %v, want 0 (River's default)", got)
	}
}

// A queued job whose name no longer exists in code must not panic the worker on the way to
// being discarded — `Work` handles that case, and `Timeout` is called BEFORE it.
func TestRiverWorker_TimeoutOfAnUnknownJobIsTheDefault(t *testing.T) {
	s := New(newFakeStore(), NewRegistry().Add(timeoutJob("known", LongJobTimeout)), nil, time.Now, testLog())
	w := &riverWorker{s: s}

	if got := w.Timeout(&river.Job[jobArgs]{Args: jobArgs{Name: "renamed-in-an-upgrade"}}); got != 0 {
		t.Errorf("unknown job ceiling = %v, want 0", got)
	}
}

// ⚠ **The ceiling must not outlive the CLAIM.** A job allowed to run past its own lease would
// keep working after its row became re-claimable, and a second worker could start the same job
// beside it — two ffmpeg passes over one catalog. Tying the two constants means the invariant is
// stated once; this asserts nobody unties them.
func TestLongJobTimeout_DoesNotOutliveTheLease(t *testing.T) {
	if LongJobTimeout > leaseHorizon {
		t.Fatalf("LongJobTimeout %v exceeds leaseHorizon %v — a job may then still be running "+
			"when its claim expires, and a second worker can pick it up", LongJobTimeout, leaseHorizon)
	}
}

// The ceiling is enforced by River, not by us, so what this pins is the WIRING: a job that
// overruns its declared ceiling has its context cancelled, and the work sees it.
func TestJobTimeout_CancelsTheWorkThatOverruns(t *testing.T) {
	var sawCancel atomic.Bool
	// Deliberately tiny: the assertion is about the plumbing, not the number.
	const tiny = 30 * time.Millisecond
	j := Job{
		Name: "overrun", Group: GroupSystem, Title: "overrun", Description: "test job overrun.", DefaultCron: everyMinute,
		Timeout: tiny,
		Run: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				sawCancel.Store(true)
			case <-time.After(2 * time.Second):
			}
			return nil
		},
	}
	s := New(newFakeStore(), NewRegistry().Add(j), nil, time.Now, testLog())

	// River owns the deadline in production; here it is applied directly so the test needs no
	// queue, no database and no clock control.
	ctx, cancel := context.WithTimeout(context.Background(), (&riverWorker{s: s}).
		Timeout(&river.Job[jobArgs]{Args: jobArgs{Name: "overrun"}}))
	defer cancel()
	s.execute(ctx, j)

	if !sawCancel.Load() {
		t.Fatal("the job ran to completion past its ceiling — its context was never cancelled")
	}
}
