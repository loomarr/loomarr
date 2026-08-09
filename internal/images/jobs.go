package images

import (
	"context"

	"github.com/mantonx/loomarr/internal/scheduler"
)

// The image service's four scheduled jobs (§22, §18.1 — V52 phase 3b).
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
		Name: "images-fetch", Title: "Download artwork",
		Description: "Downloads the posters and artwork Loomarr has noted but not yet fetched. Until this runs those images show as blurred placeholders.",
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
		Name: "images-avif", Title: "Make smaller copies of images (AVIF)",
		Description: "Re-encodes artwork into AVIF, the smallest format, so pages load faster on slow connections. It runs in the background because it is the expensive one; until a copy exists, browsers take the WebP version.",
		DefaultCron: "0 20 * * * *", ScheduleKey: "job.images_avif.schedule",
		DisabledReason: disabledReason,
		Run:            func(ctx context.Context) error { _, err := j.Run(ctx); return err },
	}
}

// RehydrateJob re-downloads artwork whose files have gone missing — the post-restore path.
//
// Daily rather than hourly because on a healthy install it finds nothing, and that is exactly why
// it has to exist: nothing else notices a missing file. The database survives a restore intact
// while `/data/images` may not, and every affected row still looks healthy from the database's
// point of view.
func RehydrateJob(f *Fetcher) scheduler.Job {
	return scheduler.Job{
		Name: "images-rehydrate", Title: "Restore missing artwork",
		Description: "Re-downloads artwork whose files are missing but can be got again. This is what repopulates your images after you restore a backup onto an empty image folder.",
		DefaultCron: "0 45 4 * * *", ScheduleKey: "job.images_rehydrate.schedule",
		Run: func(ctx context.Context) error { _, err := f.Rehydrate(ctx); return err },
	}
}

// GCJob tidies the image store.
//
// ⚠ The description leads with the six-month limit rather than with eviction, matching what the
// job actually guarantees. Eviction is a backstop at the documented envelope; the TMDB ceiling is
// a licence term, and an operator reading the Tasks page should be able to see that it is enforced.
func GCJob(g *GC) scheduler.Job {
	return scheduler.Job{
		Name: "images-gc", Title: "Tidy up stored images",
		Description: "Enforces the six-month limit on downloaded artwork, deletes images nothing uses any more, and frees disk by dropping resized copies once the image folder goes over its budget.",
		DefaultCron: "0 0 5 * * *", ScheduleKey: "job.images_gc.schedule",
		Run: func(ctx context.Context) error { _, err := g.Run(ctx); return err },
	}
}
