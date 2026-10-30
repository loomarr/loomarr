// Package fillerstructureassessment is the complete-timeline structure-assessment runtime: the
// ffmpeg media preparers that render and verify the normalized whole-source and per-window
// derivatives, the lineage records that bind those derivatives to their source, the family and
// window-set runtimes that run independent assessors serially and stitch their answers, and the
// private content-addressed evidence repositories that keep every call, stitch and decision.
//
// It sits above internal/filler and imports it for the retained-source types the split stage
// already owns (SplitSourceAsset, StructureAssessmentSource, StructureAssessmentMedia), the
// CompleteTimelineStructureDecisioner port the stage consumes, and the media helpers the preparers
// share. internal/filler knows nothing of it.
package fillerstructureassessment
