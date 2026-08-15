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
		Name: "playout-prepare", Group: scheduler.GroupPlayout, Title: "Prepare upcoming programmes",
		Description: "Pre-encodes upcoming programmes for immediate playback and keeps prepared media within its storage budget.",
		DefaultCron: "0 * * * * *", ScheduleKey: "job.playout_prepare.schedule",
		Timeout: scheduler.LongJobTimeout, DisabledReason: disabledReason,
		Run: runner.Run,
	}
}
