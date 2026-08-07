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

// fillerTranscribeJob declares the on-demand transcribe job (§10 V44) — persisting what a clip
// SAYS so the tagger can ground a brand or era for a clip whose source described it thinly.
//
// ⚠ Its own row, on the same slow cadence as the language gate and for the same reason: it shares
// the `MediaTools.Transcribe` seam (whisper ~341s per clip under QEMU), so it is expensive and
// batched. Off unless `filler.transcribe.enabled` — the job's own opt-in, read live inside Run, so
// the row stays visible on the Tasks page even when it is doing nothing (an omitted row reads as a
// job that has never failed, which is a different fact).
func fillerTranscribeJob(j *filler.TranscribeJob) scheduler.Job {
	return scheduler.Job{
		Name: "filler-transcribe", Title: "Transcribe filler clips",
		Description: "Listens to clips whose source told us almost nothing and writes down what they say, so we can work out the brand and era. Only runs on clips that need it.",
		DefaultCron: "0 15 * * * *", ScheduleKey: "job.filler_transcribe.schedule",
		Run: func(ctx context.Context) error { _, err := j.Run(ctx); return err },
	}
}

// fillerVisionJob declares the vision tagging job (§10 V44) — reading a clip's KEYFRAMES for the
// brand, category, and on-screen text a wordless spot never says out loud.
//
// ⚠ The most expensive tier and the last one, so it runs only where cheaper signals left a gap
// (especially silent clips). Off unless a vision model is configured AND `filler.vision.enabled`;
// a nil provider makes Run a no-op, so an install without one never touches ffmpeg or spends a
// multimodal token. Its own row for the same reason as its siblings.
func fillerVisionJob(j *filler.VisionJob) scheduler.Job {
	return scheduler.Job{
		Name: "filler-vision", Title: "Read filler clips' pictures",
		Description: "Looks at a few frames of clips we still can't identify — reading on-screen logos and text — to work out the brand and what they're advertising, even when nobody says it.",
		DefaultCron: "0 50 * * * *", ScheduleKey: "job.filler_vision.schedule",
		Run: func(ctx context.Context) error { _, err := j.Run(ctx); return err },
	}
}

// fillerReindexJob declares the taxonomy reindex (§10 V45a) — recomputing every clip's rolled-up tags
// from the current tag graph.
//
// ⚠ A lifecycle sibling of the media jobs above (its own Tasks-page row, off by default, read live
// inside Run) but NOT an expensive one: its body is two bulk SQL statements (rebuild the closure, then
// the rollups), no whisper/vision/ffmpeg, no per-clip loop. It exists because clip rollups are a
// DERIVED cache of (clips × graph) that goes stale when an operator edits the graph — this is the job
// that re-converges them. Its own row, like its siblings, so an operator can see it ran and connect a
// tag change to it.
func fillerReindexJob(j *filler.ReindexJob) scheduler.Job {
	return scheduler.Job{
		Name: "filler-reindex", Title: "Update clip tags to match the vocabulary",
		Description: "Recomputes every clip's rolled-up tags so they match the current tag categories. Runs after you edit the tag vocabulary yourself.",
		DefaultCron: "0 5 * * * *", ScheduleKey: "job.filler_reindex.schedule",
		Run: func(ctx context.Context) error { _, err := j.Run(ctx); return err },
	}
}
