package growth

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// Job returns the nightly fill as a scheduler job.
func (f *Fill) Job() jobs.Job {
	return jobs.Job{
		Name: "channel-growth", Group: jobs.GroupChannels, Title: "Grow channels from the library",
		Description: "Adds the library titles inside each channel's line until it reaches its target.",
		DefaultCron: "0 5 0 * * *", ScheduleKey: settings.JobGrowthSchedule.Key(),
		// Every channel is read and the library listed once; a large library takes longer than
		// the default minute.
		Timeout: 10 * time.Minute,
		Run:     func(ctx context.Context) error { _, err := f.Run(ctx); return err },
	}
}
