package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/scheduler"
)

// The filler jobs that do real media or file I/O MUST declare a Timeout — it is both their
// SIGKILL ceiling and (via scheduler.queueFor, which routes on Timeout > 0) what puts them on
// the `long` queue instead of River's 1-minute `default`. #304 gave every job here a ceiling
// but missed filler-split-sweep, which reclaims source recordings over a weeks-wide backlog and
// would silently die at 60s on a large sweep. Passing nil is safe: these constructors only wrap
// their argument in a Job and this asserts the Job's declared ceiling, never runs it.
func TestFillerMediaJobsDeclareALongTimeout(t *testing.T) {
	cases := map[string]scheduler.Job{
		"filler-sync":        fillerSyncJob(nil),
		"filler-fetch":       fillerFetchJob(nil),
		"filler-split-sweep": fillerSplitSweepJob(nil),
	}
	for name, job := range cases {
		if job.Timeout != scheduler.LongJobTimeout {
			t.Errorf("%s Timeout = %v, want scheduler.LongJobTimeout — a media/file-I/O job with no "+
				"ceiling runs under River's 1-minute default and on the `default` queue, where a large "+
				"pass is SIGKILLed and starves nothing but itself", name, job.Timeout)
		}
	}
}
