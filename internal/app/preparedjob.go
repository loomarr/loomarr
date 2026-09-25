package app

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/scheduler"
)

type preparedRunner interface{ Run(context.Context) error }

// preparedPassTimeout is the ceiling of one prepare pass. A pass only plans, launches
// planner-owned workers and starts retention, all bounded, so it must finish well inside its
// one-minute cadence. It must stay at or below River's default job timeout: a longer ceiling routes
// the job to the single-worker `long` queue, where a slow filler-pipeline pass starved it.
const preparedPassTimeout = 45 * time.Second

// preparedPlayoutJob lives at composition because prepared cannot import scheduler: scheduler
// imports store, whose filler media pipeline reaches playout and would close an import cycle.
func preparedPlayoutJob(runner preparedRunner, disabledReason string) scheduler.Job {
	return scheduler.Job{
		Name: "playout-prepare", Group: scheduler.GroupPlayout, Title: "Prepare upcoming programmes",
		Description: "Pre-encodes upcoming programmes for immediate playback and keeps prepared media within its storage budget.",
		DefaultCron: "0 * * * * *", ScheduleKey: "job.playout_prepare.schedule",
		Timeout: preparedPassTimeout, DisabledReason: disabledReason,
		Run: runner.Run,
	}
}
