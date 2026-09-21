package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/scheduler"
)

// The filler jobs that do real media or file I/O MUST declare a Timeout — it is both their
// SIGKILL ceiling and (via scheduler.queueFor, which routes on Timeout > 0) what puts them on
// the `long` queue instead of River's 1-minute `default`. #304 gave every job here a ceiling
// but missed filler-split-sweep, which reclaims source recordings over a weeks-wide backlog and
// would silently die at 60s on a large sweep. Passing nil is safe: these constructors only wrap
// their argument in a Job and this asserts the Job's declared ceiling, never runs it.
func TestFillerMediaJobsDeclareALongTimeout(t *testing.T) {
	cases := map[string]scheduler.Job{
		"filler-sync":        fillerSyncJob(nil),
		"filler-fetch":       fillerFetchJob(nil),
		"filler-pipeline":    fillerPipelineJob(nil),
		"filler-split-sweep": fillerSplitSweepJob(nil),
	}
	for name, job := range cases {
		if job.Timeout != scheduler.LongJobTimeout {
			t.Errorf("%s Timeout = %v, want scheduler.LongJobTimeout — a media/file-I/O job with no "+
				"ceiling runs under River's 1-minute default and on the `default` queue, where a large "+
				"pass is SIGKILLed and starves nothing but itself", name, job.Timeout)
		}
	}
}

func TestFillerPipelineDriverRunsDetailsAfterPreparation(t *testing.T) {
	var order []string
	driver := fillerPipelineDriver{
		prepare: func(context.Context) (filler.PipelineResult, error) {
			order = append(order, "prepare")
			return filler.PipelineResult{}, errors.New("preparation failed")
		},
		details: func(context.Context) error {
			order = append(order, "details")
			return errors.New("details failed")
		},
	}
	err := driver.Run(t.Context())
	if strings.Join(order, ",") != "prepare,details" {
		t.Fatalf("order = %v", order)
	}
	if err == nil || !strings.Contains(err.Error(), "preparation failed") || !strings.Contains(err.Error(), "details failed") {
		t.Fatalf("joined error = %v", err)
	}
}

func TestFillerPipelineDriverStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	detailsRan := false
	driver := fillerPipelineDriver{
		prepare: func(context.Context) (filler.PipelineResult, error) {
			cancel()
			return filler.PipelineResult{}, nil
		},
		details: func(context.Context) error { detailsRan = true; return nil },
	}
	if err := driver.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", err)
	}
	if detailsRan {
		t.Fatal("details ran after the scheduler lease was cancelled")
	}
}

func TestFillerPipelineDriverDefersDetailsWhilePreparationCanAdvance(t *testing.T) {
	detailsRan := false
	driver := fillerPipelineDriver{
		prepare: func(context.Context) (filler.PipelineResult, error) {
			return filler.PipelineResult{Overview: filler.PipelineOverview{Runnable: 4}}, nil
		},
		details: func(context.Context) error {
			detailsRan = true
			return nil
		},
	}
	if err := driver.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if detailsRan {
		t.Fatal("optional details ran while playable preparation still had runnable clips")
	}
}

func TestFillerFetchJobUsesOnlyAnInternalWakeSchedule(t *testing.T) {
	job := fillerFetchJob(nil)
	if job.DefaultCron != "0 * * * * *" {
		t.Fatalf("filler fetch wake = %q, want the fixed one-minute internal wake", job.DefaultCron)
	}
	if job.ScheduleKey != "" {
		t.Fatalf("filler fetch schedule key = %q, want no operator cron authority", job.ScheduleKey)
	}
}
