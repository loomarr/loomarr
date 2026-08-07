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

// fillerLanguageJob declares the language gate (§10 V40) — detecting what a clip's speech is in
// and removing the ones that are confidently not `filler.language`.
//
// ⚠ **Its own job, and on a much slower cron than its siblings**, because it is the expensive one.
// On the local backend a clip costs ~3s natively and ~341s under QEMU, so a batch of 25 is
// minutes-to-hours of work depending on the machine — nothing like the folder read `filler-sync`
// does. Hourly drains a catalog steadily without a pass ever overlapping the next.
//
// ⚠ It is also the only filler job that DELETES something. A separate row on the Tasks page is
// what lets an operator see it ran, pause it, and connect "my Spanish advert disappeared" to a job
// rather than to a mystery.
func fillerLanguageJob(j *filler.LanguageJob) scheduler.Job {
	return scheduler.Job{
		Name: "filler-language", Title: "Check filler languages",
		Description: "Listens to a few seconds of each new clip and removes the ones spoken in a different language. Clips with no speech — music or visuals only — are always kept.",
		DefaultCron: "0 30 * * * *", ScheduleKey: "job.filler_language.schedule",
		Run: func(ctx context.Context) error { _, err := j.Run(ctx); return err },
	}
}

// fillerSplitJob declares the scheduled compilation split (§10 V43).
//
// ⚠ **This job PROPOSES; it does not confirm.** That separation is what lets it be on by
// default: an unconfirmed proposal writes no clips and consumes no file, so the cost of it
// running on a recording the operator did not care about is a review they ignore. Confirming
// without a human is the separate opt-in behind `filler.autosplit.enabled`.
//
// ⚠ Its own row on the Tasks page, like the fetch, and for the same reason: detection is minutes
// per file and can fail per-recording. Folding it into the sync would report "the filler catalog
// sync failed" when what actually happened is that whisper could not read one file — and an
// operator pausing the sync to stop the CPU load would also stop their dropped-in clips being
// noticed.
//
// ⚠ Hourly rather than every 15 minutes, unlike the sync. This one is expensive (ffmpeg, then
// whisper) and bounded to a few recordings per pass, so a backlog drains over cycles by design.
func fillerSplitJob(r *filler.SplitRunner) scheduler.Job {
	return scheduler.Job{
		Name: "filler-split", Title: "Find adverts inside long recordings",
		Description: "Looks through long recordings in your catalog and works out where each advert inside them starts and ends, so they're ready for you to check under Filler → Incoming.",
		DefaultCron: "0 45 * * * *", ScheduleKey: "job.filler_split.schedule",
		Run: func(ctx context.Context) error { _, err := r.Run(ctx); return err },
	}
}
