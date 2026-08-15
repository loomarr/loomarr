package app

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/scheduler"
)

type noopPreparedRunner struct{}

func (noopPreparedRunner) Run(context.Context) error { return nil }

func TestPreparedPlayoutJobIsVisibleLongMediaWork(t *testing.T) {
	job := preparedPlayoutJob(noopPreparedRunner{}, "")
	if job.Name != "playout-prepare" || job.ScheduleKey != "job.playout_prepare.schedule" ||
		job.DefaultCron != "0 * * * * *" || job.Timeout != scheduler.LongJobTimeout {
		t.Fatalf("job = %+v", job)
	}
}
