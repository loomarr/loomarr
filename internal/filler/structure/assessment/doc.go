// Package assessment is the complete-timeline structure-assessment runtime: the
// ffmpeg media preparer that renders and verifies the normalized per-window derivatives, the
// lineage records that bind those derivatives to their source, the family and window-set runtimes
// that run independent assessors serially and stitch their answers, and the private
// content-addressed evidence repositories that keep every call, stitch and decision.
//
// It sits above internal/filler and internal/filler/split, importing the latter for the
// retained-source types the split stage already owns (split.SourceAsset, split.AssessmentSource)
// and the split.CompleteTimelineDecisioner port the stage consumes, and the former for the media
// helpers the preparer shares. internal/filler knows nothing of it.
package assessment
