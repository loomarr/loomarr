package reconcile

import (
	"context"

	"github.com/loomarr/loomarr/internal/scheduler"
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
		Name: "reconcile", Group: scheduler.GroupAcquisitions, Title: "Reconcile acquisitions",
		Description: "Advances approved requests through download, retry, and completion states.",
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
			Name: "library-scan", Group: scheduler.GroupAcquisitions, Title: "Scan recent library additions",
			Description: "Marks approved titles as available when they appear among recent media-server additions.",
			DefaultCron: "0 */5 * * * *", ScheduleKey: "job.library_scan.schedule",
			Run: func(ctx context.Context) error { _, err := s.Incremental(ctx); return err },
		},
		{
			Name: "library-full-scan", Group: scheduler.GroupAcquisitions, Title: "Scan the full library",
			Description: "Checks the entire media-server library as a daily safety net for missed additions.",
			DefaultCron: "0 0 3 * * *", ScheduleKey: "job.library_full_scan.schedule",
			Run: func(ctx context.Context) error { _, err := s.Full(ctx); return err },
		},
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
		Name: name, Group: scheduler.GroupAcquisitions, Title: title, Description: description,
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
