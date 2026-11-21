package app

import (
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
	LibraryFolderSummarized(s scan.FolderSummary)
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
