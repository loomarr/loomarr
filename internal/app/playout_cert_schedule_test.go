package app

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

func TestSyntheticProgrammeScheduleHasSuccessiveCurrentNextAndDiscontinuity(t *testing.T) {
	epoch := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	programmeSchedule := syntheticProgrammeSchedule{epoch: epoch, duration: 6 * time.Second}
	current, next, offset := programmeSchedule.airings(epoch.Add(18*time.Second), "channel-a")
	if current.StartedAt != epoch.Add(18*time.Second) || current.EndsAt != epoch.Add(24*time.Second) || next.StartedAt != current.EndsAt || next.EndsAt != epoch.Add(30*time.Second) || offset != 0 {
		t.Fatalf("exact boundary airings current=%+v next=%+v offset=%s", current, next, offset)
	}
	if current.ScheduleBlockID != "synthetic-block-3" || next.ScheduleBlockID != "synthetic-block-4" || current.ContentID == next.ContentID || current.Kind != schedule.SlotProgram {
		t.Fatalf("successive identity/discontinuity = current=%+v next=%+v", current, next)
	}
	current, next, offset = programmeSchedule.airings(epoch.Add(23*time.Second), "channel-a")
	if current.StartedAt != epoch.Add(18*time.Second) || next.StartedAt != epoch.Add(24*time.Second) || offset != 5*time.Second {
		t.Fatalf("mid-epoch current/next/offset = current=%+v next=%+v offset=%s", current, next, offset)
	}
}
