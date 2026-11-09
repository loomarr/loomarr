package demolibrary

import (
	"testing"
	"time"
)

// A demo programme shorter than a half-hour slot turns the 4-hour guide into slivers with no room
// for a label (#2747). Every invented programme format runs at least 22 minutes, and the 1080p
// film a full 90.
func TestProgrammeFormatsAreLongEnoughForTheGuide(t *testing.T) {
	for _, f := range Formats {
		if got := time.Duration(f.Duration) * time.Second; got < 22*time.Minute {
			t.Errorf("%s runs %s; the guide needs at least 22 minutes to label a block at 4h", f.ID, got)
		}
		if f.ID == "film-1080p-h264" && f.Duration != 90*60 {
			t.Errorf("%s runs %ds, want a 90-minute film", f.ID, f.Duration)
		}
	}
}
