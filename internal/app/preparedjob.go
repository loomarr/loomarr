package app

import (
	"context"

	"github.com/mantonx/loomarr/internal/scheduler"
)

type preparedRunner interface{ Run(context.Context) error }

// preparedPlayoutJob lives at composition because prepared cannot import scheduler: scheduler
// imports store, whose filler media pipeline reaches playout and would close an import cycle.
func preparedPlayoutJob(runner preparedRunner, disabledReason string) scheduler.Job {
	return scheduler.Job{
		Name: "playout-prepare", Title: "Prepare upcoming channel programmes",
		Description: "Prepares the next programmes shared across your channels so changing channels starts at the live point without waiting for a new encoder.",
		DefaultCron: "0 * * * * *", ScheduleKey: "job.playout_prepare.schedule",
		Timeout: scheduler.LongJobTimeout, DisabledReason: disabledReason,
		Run: runner.Run,
	}
}
