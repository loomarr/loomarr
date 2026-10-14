package images

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/scheduler"
)

// The image service's scheduled jobs (§22, §18.1).
//
// ⚠ These live here rather than in internal/app, unlike the filler jobs. The exception there is
// structural: `store` imports `filler` for the Clip type, so `filler` importing `scheduler` (which
// imports `store`) is an import cycle. Nothing imports this package, so the normal convention —
// a subsystem declares its own jobs, as channels, auth, reconcile and recurate all do — applies.
//
// Every ScheduleKey below is declared in internal/settings. ⚠ That coupling is not optional: the
// settings service PANICS at startup on Resolve of an undeclared key, so a job registered without
// its row takes the whole install down rather than degrading. The four keys landed in phase 3a
// precisely so this file could not be the thing that broke boot.

// FetchJob pulls the bytes for artwork that has been recorded but never downloaded.
//
// ⚠ Every minute, which is far tighter than any other outbound job here, and affordable for the
// same reason it is necessary. `Adopt` deliberately returns before the bytes exist, so until this
// runs a surface shows a placeholder — the delay is directly visible to a person looking at the
// guide. A pass costs one indexed SELECT on an idle install.
func FetchJob(f *Fetcher) scheduler.Job {
	return scheduler.Job{
		// Network downloads, N per pass. One slow upstream is enough to pass River's 1m default,
		// and a SIGKILL mid-fetch leaves a half-written file rather than a recorded miss.
		Timeout: scheduler.LongJobTimeout,
		Name:    "images-fetch", Group: scheduler.GroupArtwork, Title: "Download pending artwork",
		Description: "Downloads recorded artwork that is still showing as a placeholder.",
		DefaultCron: "0 * * * * *", ScheduleKey: "job.images_fetch.schedule",
		Run: func(ctx context.Context) error { _, err := f.FetchPending(ctx); return err },
	}
}

// AVIFJobSpec declares the AVIF encoder pass.
//
// `disabledReason` states a fact about the ENVIRONMENT — an ffmpeg with no libaom-av1 cannot
// produce this format however the install is configured. ⚠ It is not an operator off switch, and
// the job is registered even when set: an absent Tasks row is indistinguishable from a job that
// runs fine and has never failed, which is the ambiguity §18.1 added DisabledReason to remove.
func AVIFJobSpec(j *AVIFJob, disabledReason string) scheduler.Job {
	return scheduler.Job{
		// AVIF encoding is CPU- and memory-intensive over a batch, so it must not inherit
		// River's one-minute ceiling.
		Timeout: scheduler.LongJobTimeout,
		Name:    "images-avif", Group: scheduler.GroupArtwork, Title: "Encode AVIF artwork",
		Description: "Creates smaller AVIF variants while retaining WebP fallbacks for browser compatibility.",
		DefaultCron: "0 20 * * * *", ScheduleKey: "job.images_avif.schedule",
		DisabledReason: disabledReason,
		Run:            func(ctx context.Context) error { _, err := j.Run(ctx); return err },
	}
}

// MaintenanceJob restores missing originals and then applies the image-store retention policy.
// They share one daily outcome: keep durable artwork available while bounding storage. Both stages
// run even when one fails, so a transient provider error cannot prevent compliance cleanup.
func MaintenanceJob(f *Fetcher, g *GC) scheduler.Job {
	return scheduler.Job{
		Name: "images-maintenance", Group: scheduler.GroupArtwork, Title: "Maintain stored artwork",
		Description: "Restores recoverable files, enforces retention, removes unreferenced images, and evicts variants above budget.",
		DefaultCron: "0 0 5 * * *", ScheduleKey: "job.images_maintenance.schedule",
		Run: func(ctx context.Context) error {
			_, restoreErr := f.Rehydrate(ctx)
			_, cleanupErr := g.Run(ctx)
			return errors.Join(restoreErr, cleanupErr)
		},
	}
}

// AdoptJobSpec declares the clip-artwork adoption pass (§22, V52 phase 6).
//
// ⚠ Every five minutes rather than hourly, because this is the step between a clip being rendered
// and its artwork being VISIBLE through the image service. Until it runs, a freshly-scanned clip
// shows through the legacy route (the migration window) — an hour of that on every import would
// read as the feature not working.
//
// ⚠ Its work list empties on a healthy install, which is what makes the cadence cheap: the query
// selects only clips that have artwork on disk and no image identity yet, so a steady state costs
// one indexed query and nothing else.
func AdoptJobSpec(j *AdoptJob) scheduler.Job {
	return scheduler.Job{
		Name: "images-adopt-artwork", Group: scheduler.GroupArtwork, Title: "Adopt clip artwork",
		Description: "Moves clip thumbnails and previews into the shared image library for consistent delivery.",
		DefaultCron: "0 */5 * * * *", ScheduleKey: "job.images_adopt_artwork.schedule",
		Run: func(ctx context.Context) error { _, err := j.Run(ctx); return err },
	}
}
