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
		Description: "Re-reads your filler folder so new commercials and bumpers become available to the ad breaks between programmes.",
		DefaultCron: "0 */15 * * * *", ScheduleKey: "job.filler_sync.schedule",
		Run: func(ctx context.Context) error { _, err := s.Sync(ctx); return err },
	}
}

// fillerFetchJob declares auto-fetch (§10 V38b) — polling registered sources for new clips.
//
// ⚠ **Its own job rather than a step inside the sync**, because the two have different failure
// modes and different appetites. The sync reads a local folder and is cheap; this one makes
// outbound requests and downloads. Folding it in would mean an archive.org outage showing up as
// "the filler catalog sync failed", and an operator pausing the sync to stop the downloading
// would also stop their own dropped-in files being noticed.
//
// A separate row on the Tasks page also means the fetch has its own last-run and its own error,
// which is what an operator asking "why is nothing arriving?" actually needs to see.
func fillerFetchJob(f *filler.Fetcher) scheduler.Job {
	return scheduler.Job{
		Name: "filler-fetch", Title: "Fetch new filler clips",
		Description: "Checks the sources you've added for new commercials and downloads them. Everything fetched waits under Filler → Incoming until it's checked.",
		DefaultCron: "0 0 */6 * * *", ScheduleKey: "job.filler_fetch.schedule",
		Run: func(ctx context.Context) error { _, err := f.Run(ctx); return err },
	}
}
