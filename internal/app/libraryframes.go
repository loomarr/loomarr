package app

import (
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/library/scan"
)

// Library frame pacing (#2151). A scan advances many times a second and writes titles in bursts of
// thousands; every browser open on the Library would otherwise get a frame for each.
const (
	// libraryScanFrameGap is the least time between library_scan frames while a step runs.
	libraryScanFrameGap = time.Second
	// libraryTitlesFrameGap is the least time between library_titles frames.
	libraryTitlesFrameGap = 2 * time.Second
)

// libraryFrames publishes the Library's live frames (eventEmitter).
type libraryFrames interface {
	LibraryScanChanged(p scan.ProgressSnapshot)
	LibraryTitlesChanged()
	LibraryRootsChanged()
}

// libraryFramesFor is the Tracker that paces library_scan frames from what it records, and the
// TitlesChanged hook that paces library_titles frames. frames nil publishes nothing.
func libraryFramesFor(frames libraryFrames) (*scan.Tracker, func()) {
	tracker := scan.NewTracker(time.Now, nil)
	if frames == nil {
		return tracker, nil
	}
	scanFrames := newFrameThrottle(libraryScanFrameGap, func() { frames.LibraryScanChanged(tracker.Snapshot(time.Now())) })
	tracker.OnChange = scanFrames.poke
	titleFrames := newFrameThrottle(libraryTitlesFrameGap, frames.LibraryTitlesChanged)
	return tracker, func() { titleFrames.poke(false) }
}

// frameThrottle publishes a latest-wins frame at most once per gap. A change inside the gap
// schedules one trailing frame at its end, so the last change is always published; publish reads
// the state when it runs, so that frame carries the latest. An urgent change publishes at once.
type frameThrottle struct {
	gap     time.Duration
	publish func()
	now     func() time.Time
	// after runs f once d has passed; time.AfterFunc outside tests.
	after func(d time.Duration, f func())

	mu       sync.Mutex
	last     time.Time
	trailing bool
	// sending orders the frames: each takes its snapshot and posts it in turn, so the last frame
	// out carries the latest state even when a trailing frame races an urgent one.
	sending sync.Mutex
}

func (t *frameThrottle) send() {
	t.sending.Lock()
	defer t.sending.Unlock()
	t.publish()
}

func newFrameThrottle(gap time.Duration, publish func()) *frameThrottle {
	return &frameThrottle{
		gap: gap, publish: publish, now: time.Now,
		after: func(d time.Duration, f func()) { time.AfterFunc(d, f) },
	}
}

// poke notes a change. It never blocks on the frame's subscribers: the bus drops for a slow one.
func (t *frameThrottle) poke(urgent bool) {
	t.mu.Lock()
	now := t.now()
	switch {
	case urgent || (!t.trailing && now.Sub(t.last) >= t.gap):
		t.last = now
		t.mu.Unlock()
		t.send()
	case t.trailing:
		// The trailing frame already scheduled carries this change.
		t.mu.Unlock()
	default:
		t.trailing = true
		wait := t.gap - now.Sub(t.last)
		t.mu.Unlock()
		t.after(wait, t.fire)
	}
}

// fire publishes the trailing frame.
func (t *frameThrottle) fire() {
	t.mu.Lock()
	t.trailing, t.last = false, t.now()
	t.mu.Unlock()
	t.send()
}
