package reconcile

import (
	"context"

	"github.com/mantonx/loomarr/internal/scheduler"
)

// Each service declares its OWN scheduler job (§18.1).
//
// ⚠ **The cron default, the settings key and the body belong next to the code they
// schedule.** They used to live in the composition root, which meant every job's schedule
// was stated in a file that knows nothing about what the job does — and changing a cadence
// meant editing `app.go` rather than the package that owns the work.
//
// The root still decides WHICH sets apply to this build (there is no store ⇒ no jobs at
// all; arr XOR seerr for the queue poll). That is genuinely its decision, and it stays there.
//
// These are methods rather than free functions on purpose: the service is the thing that
// knows how to run, so a Job without its service cannot be constructed by accident.

// Job returns the reconcile tick as a scheduler job — the deadline/retry backstop (§4).
func (r *Reconciler) Job() scheduler.Job {
	return scheduler.Job{
		Name: "reconcile", Title: "Reconcile downloads",
		Description: "Checks titles you approved that are still downloading, and retries or gives up on ones that have stalled past their deadline.",
		DefaultCron: "0 */5 * * * *", ScheduleKey: "job.reconcile.schedule",
		Run: func(ctx context.Context) error { _, err := r.Tick(ctx); return err },
	}
}

// Jobs returns the library scan's two jobs.
//
// ⚠ Both, together, because they are one policy split across two cadences: Incremental
// runs often over a recent window, Full is the daily safety net for whatever that window
// missed. Registering one without the other is a misconfiguration that would look like it
// worked.
func (s *LibraryScan) Jobs() []scheduler.Job {
	return []scheduler.Job{
		{
			Name: "library-scan", Title: "Scan library for new titles",
			Description: "Looks at what your media server added recently and marks any approved title that has landed as available to schedule.",
			DefaultCron: "0 */5 * * * *", ScheduleKey: "job.library_scan.schedule",
			Run: func(ctx context.Context) error { _, err := s.Incremental(ctx); return err },
		},
		{
			Name: "library-full-scan", Title: "Full library sweep",
			Description: "Checks your whole library rather than just recent additions, as a safety net for anything the frequent scan missed.",
			DefaultCron: "0 0 3 * * *", ScheduleKey: "job.library_full_scan.schedule",
			Run: func(ctx context.Context) error { _, err := s.Full(ctx); return err },
		},
	}
}

// Job returns the series episode refresh (§5, §18.1).
//
// Deliberately NOT folded into library-scan: that job only correlates in-flight
// acquisitions and returns early when there are none, so it would never revisit an
// already-available show — the exact set this refreshes.
func (e *EpisodeRefresh) Job() scheduler.Job {
	return scheduler.Job{
		Name: "series-episode-refresh", Title: "Refresh series episode lists",
		Description: "Re-reads the episode list for shows your channels play, so newly added episodes start airing without waiting for a rebuild.",
		DefaultCron: "0 0 * * * *", ScheduleKey: "job.series_episode_refresh.schedule",
		Run: func(ctx context.Context) error { _, err := e.Run(ctx); return err },
	}
}

// Job returns the queue poller under the given name and title (§18.1).
//
// ⚠ **The NAME is a parameter, and that is not indecision.** One poller type serves two
// mutually exclusive providers — the direct arr reads Sonarr/Radarr queues for real
// byte-progress, Seerr reads /media for a coarse status — and the Tasks page must say which
// one this install actually polls. A single fixed name would tell an operator running Seerr
// that Loomarr is polling Sonarr.
//
// The two are mutually exclusive (a provider is arr XOR seerr), so the names never collide.
func (q *QueuePoll) Job(name, title, description string) scheduler.Job {
	return scheduler.Job{
		Name: name, Title: title, Description: description,
		DefaultCron: "0 * * * * *", ScheduleKey: "job." + jobKey(name) + ".schedule",
		Run: func(ctx context.Context) error { _, err := q.Poll(ctx); return err },
	}
}

// jobKey converts a job name to its settings-key segment ("arr-queue-poll" →
// "arr_queue_poll"), so the two spellings cannot drift apart by hand.
func jobKey(name string) string {
	out := []rune(name)
	for i, r := range out {
		if r == '-' {
			out[i] = '_'
		}
	}
	return string(out)
}
