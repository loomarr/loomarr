package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/scheduler"
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
		// A folder walk that hashes every file it finds, so it scales with the catalog rather
		// than with what changed — minutes on a large drop folder, well past River's 1m default.
		Timeout: scheduler.LongJobTimeout,
		Name:    "filler-sync", Group: scheduler.GroupFiller, Title: "Sync the filler catalogue",
		Description: "Scans configured folders and libraries so new filler becomes available for channel breaks.",
		DefaultCron: "0 */15 * * * *", ScheduleKey: "job.filler_sync.schedule",
		Run: func(ctx context.Context) error { _, err := s.Sync(ctx); return err },
	}
}

type fillerEnrichmentRunner interface {
	Run(context.Context) (fillerenrichment.RunResult, error)
}

type fillerPipelineRunner interface {
	Run(context.Context) error
}

// fillerPipelineDriver owns one operator-facing background loop while preserving two independent
// authority boundaries inside it: readiness prepares clips for playback; details enrich clips
// that are already ready. A preparation failure does not strand the ready backlog, but a cancelled
// lease stops before beginning more work.
type fillerPipelineDriver struct {
	prepare func(context.Context) (filler.PipelineResult, error)
	details func(context.Context) error
	// deferred counts consecutive passes that skipped details because preparation was advancing.
	deferred int
}

// fillerDetailMaxDeferrals bounds how many consecutive advancing passes may postpone details.
// ⚠ Deferral used to key on "anything is Runnable or InProgress", which is a property of the
// backlog, not of progress: a backlog that never drains (production held 202 clips, 34 ready)
// starved enrichment forever and filler_enrichment_passes stayed empty (#1412).
const fillerDetailMaxDeferrals = 3

func newFillerPipelineDriver(pipeline *filler.Pipeline, details fillerEnrichmentRunner, log *slog.Logger) *fillerPipelineDriver {
	return &fillerPipelineDriver{
		prepare: func(ctx context.Context) (filler.PipelineResult, error) {
			if pipeline == nil {
				return filler.PipelineResult{}, nil
			}
			return pipeline.RunOnce(ctx)
		},
		details: func(ctx context.Context) error {
			if details == nil {
				return nil
			}
			result, err := details.Run(ctx)
			if log != nil {
				log.Info("filler details run", "considered", result.Considered,
					"updated", result.Updated, "failed", result.Failed)
			}
			return err
		},
	}
}

func (d *fillerPipelineDriver) Run(ctx context.Context) error {
	var prepared filler.PipelineResult
	var prepareErr error
	if d.prepare != nil {
		prepared, prepareErr = d.prepare(ctx)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(prepareErr, err)
	}
	// Descriptive enrichment is optional and may involve a remote model. While preparation is
	// actually advancing clips, let the next bounded pass keep moving them toward playable
	// readiness — but only for a bounded number of passes, and never on the strength of a backlog
	// that did not advance. Scheduled retries do not block enrichment because they cannot advance.
	preparing := prepared.Advanced > 0 && (prepared.Overview.Runnable > 0 || prepared.Overview.InProgress > 0)
	if preparing && d.deferred < fillerDetailMaxDeferrals {
		d.deferred++
		return prepareErr
	}
	d.deferred = 0
	var detailsErr error
	if d.details != nil {
		detailsErr = d.details(ctx)
	}
	return errors.Join(prepareErr, detailsErr)
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
		// yt-dlp downloading real files off the network. A single clip can exceed a minute on
		// its own, and this walks every registered source.
		Timeout: scheduler.LongJobTimeout,
		Name:    "filler-fetch", Group: scheduler.GroupFiller, Title: "Fetch new filler",
		Description: "Checks sources whose automatic-download schedule is due and adds new clips to Incoming.",
		// Fixed, cheap due-source wake. The user cadence lives only in filler.fetch.every and the
		// per-source policy; an idle pass makes no provider request.
		DefaultCron: "0 * * * * *",
		Run:         func(ctx context.Context) error { _, err := f.Run(ctx); return err },
	}
}

// fillerPipelineJob declares the ingest pipeline (§10 V51b) — the ONE driver that replaced
// `filler-language`, `filler-split`, `filler-transcribe` and `filler-vision`.
//
// ⚠ **Four rows became one deliberately, reversing the "its own row" reasoning above, and it is
// worth saying why the earlier argument no longer holds.** Each of those jobs was its own row so
// an operator could see it ran, pause it, and connect an effect to a cause. That works when the
// jobs are independent. They were not: they operated on the same clips, in an order nobody owned,
// and the thing an operator actually wanted to see — *where is this clip up to?* — was the one
// question four job rows could not answer, because it lives across all of them. The per-clip
// LADDER answers it, so the visibility moved to where the work is rather than being deleted.
//
// Pausing is preserved and is now more honest: pausing this row stops the whole ingest, which is
// what pausing "the expensive one" was usually trying to do anyway.
//
// ⚠ Every TWO MINUTES, far more often than the hourly sweeps it replaces, and that is affordable
// for the same reason it is necessary. A pass is bounded by the budget rather than by the catalog,
// so an idle install does one cheap work-list query; a busy one advances the next clip promptly
// instead of leaving a fresh download sitting for up to an hour. The sweeps had to be rare because
// each one re-read the entire catalog.
func fillerPipelineJob(runner fillerPipelineRunner) scheduler.Job {
	return scheduler.Job{
		// ⚠ **`Timeout` is not optional on this job.** It runs ffmpeg and whisper, and River's
		// default ceiling is ONE MINUTE — under which a single `blackdetect`/`silencedetect`
		// pass over a 20-minute recording (measured at 40s alone) is SIGKILLed part-way, and
		// the operator sees `signal: killed` rather than anything about time. Matched to
		// `scheduler.LongJobTimeout`, which is the lease horizon: a job may run right up to the
		// point its claim would expire, and no further.
		Timeout: scheduler.LongJobTimeout,
		Name:    "filler-pipeline", Group: scheduler.GroupFiller, Title: "Prepare new filler",
		Description: "Prepares incoming clips for playback, then fills in useful details for clips that are ready.",
		DefaultCron: "0 */2 * * * *", ScheduleKey: "job.filler_pipeline.schedule",
		Run: func(ctx context.Context) error {
			if runner == nil {
				return nil
			}
			return runner.Run(ctx)
		},
	}
}

// The split-review sweep (§10 V54): expired proposals are retired and their recordings reclaimed.
//
// ⚠ **`Timeout` is not optional here** — the sweep retires proposals and reclaims their source
// recordings, real file I/O that over a large backlog exceeds River's one-minute default. The
// ceiling also routes the job to the `long` queue (derived from `Timeout > 0`, `scheduler.queueFor`).
// #304 gave every other job in this file its ceiling but left this one under the default; a stale
// note here claimed the fix "cannot declare them yet, both land in #304" long after #304 had merged.
//
// ⚠ Daily, deliberately off-peak, and NOT more often. The window is measured in weeks, so a faster
// cadence buys nothing and only widens the chance of a pass landing while an operator is mid-review
// on a reel that is one hour past its expiry.
func fillerSplitSweepJob(sw *filler.SplitSweeper) scheduler.Job {
	return scheduler.Job{
		Timeout: scheduler.LongJobTimeout,
		Name:    "filler-split-sweep", Group: scheduler.GroupFiller, Title: "Expire split suggestions",
		Description: "Removes expired split proposals and their source recordings while retaining produced clips.",
		DefaultCron: "0 45 4 * * *", ScheduleKey: "job.filler_split_sweep.schedule",
		Run: func(ctx context.Context) error { _, err := sw.Run(ctx); return err },
	}
}
