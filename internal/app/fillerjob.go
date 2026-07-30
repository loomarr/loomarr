package app

import (
	"context"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/scheduler"
)

// fillerSyncJob declares the filler catalog sync (§10, §18.1).
//
// ⚠ **This one job cannot live with its subsystem, and the reason is structural rather than
// stylistic.** `internal/filler` may never import `internal/scheduler`: the scheduler
// imports `store`, and `store` imports `filler` for the Clip type — so a `Job()` method on
// Syncer is an import cycle, not a preference. Verified by trying it.
//
// So the declaration sits here, in the one package that may import both, and stays as close
// to the shape its siblings use as the cycle allows. If `store` ever stops depending on
// `filler`, this belongs in `internal/filler/jobs.go` beside the rest.
func fillerSyncJob(s *filler.Syncer) scheduler.Job {
	return scheduler.Job{
		Name: "filler-sync", Title: "Sync filler catalog",
		DefaultCron: "0 */15 * * * *", ScheduleKey: "job.filler_sync.schedule",
		Run: func(ctx context.Context) error { _, err := s.Sync(ctx); return err },
	}
}
