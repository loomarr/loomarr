package app

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/scan"
)

// fakeThrottle is a frameThrottle on a settable clock whose trailing frame fires when the test
// says.
func fakeThrottle(gap time.Duration) (*frameThrottle, *int, *time.Time, *[]func()) {
	published := 0
	at := time.Unix(1_900_000_000, 0)
	var timers []func()
	t := newFrameThrottle(gap, func() { published++ })
	t.now = func() time.Time { return at }
	t.after = func(_ time.Duration, f func()) { timers = append(timers, f) }
	return t, &published, &at, &timers
}

// A burst of changes publishes one frame at once and one trailing frame at the end of the gap,
// which carries the last change.
func TestFrameThrottleBurstGivesOneFrameNowAndOneTrailing(t *testing.T) {
	th, published, at, timers := fakeThrottle(2 * time.Second)
	for range 50 {
		th.poke(false)
		*at = at.Add(10 * time.Millisecond)
	}
	if *published != 1 || len(*timers) != 1 {
		t.Fatalf("published %d with %d trailing frames scheduled, want 1 and 1", *published, len(*timers))
	}
	*at = at.Add(2 * time.Second)
	(*timers)[0]()
	if *published != 2 {
		t.Fatalf("published %d after the trailing frame, want 2", *published)
	}
	// Quiet since: the next change goes out at once.
	*at = at.Add(3 * time.Second)
	th.poke(false)
	if *published != 3 || len(*timers) != 1 {
		t.Fatalf("published %d, %d timers; want the next change at once", *published, len(*timers))
	}
}

// An urgent change, a step beginning or ending or a pause, is never held back.
func TestFrameThrottleUrgentPublishesAtOnce(t *testing.T) {
	th, published, _, _ := fakeThrottle(time.Second)
	th.poke(false)
	th.poke(true)
	th.poke(true)
	if *published != 3 {
		t.Fatalf("published %d, want every urgent change at once", *published)
	}
}

// The Tracker paces library_scan frames; the titles hook paces library_titles frames.
func TestLibraryFramesFor(t *testing.T) {
	frames := &recordedFrames{}
	tracker, titles := libraryFramesFor(frames)
	tracker.Begin(scan.StepWalking, 3)
	tracker.End(scan.StepWalking)
	if len(frames.scans) != 2 || frames.scans[0].Step != scan.StepWalking || frames.scans[1].Step != "" {
		t.Fatalf("scan frames = %+v, want walking then idle", frames.scans)
	}
	titles()
	if frames.titles != 1 {
		t.Fatalf("titles frames = %d, want 1", frames.titles)
	}
	if tracker, titles := libraryFramesFor(nil); tracker == nil || titles != nil {
		t.Fatal("with no frames, the Tracker still records and no hook is set")
	}
}

type recordedFrames struct {
	scans  []scan.ProgressSnapshot
	titles int
	roots  int
}

func (r *recordedFrames) LibraryScanChanged(p scan.ProgressSnapshot) { r.scans = append(r.scans, p) }
func (r *recordedFrames) LibraryTitlesChanged()                      { r.titles++ }
func (r *recordedFrames) LibraryRootsChanged()                       { r.roots++ }
func (r *recordedFrames) LibraryFolderSummarized(scan.FolderSummary) {}
