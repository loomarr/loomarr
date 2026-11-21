package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/library/scan"
)

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
