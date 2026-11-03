// Package fillerscreening is the rendered-child screening runtime: the four-axis coordinator that
// records one closed, replayable result per axis for an immutable child subject, the private
// content-addressed evidence repository behind it, the playback-integrity and qualification
// evaluators, the release certification that replays an aggregate against its authority, and the
// SCREEN conveyor stage that turns the outcome into a pipeline verdict.
//
// It sits above internal/filler and imports it for the subject and aggregate types the sidecar
// embeds, the clip catalog types and the stage contract; internal/filler knows nothing of it. The
// types the sidecar embeds (SegmentScreeningSubject, SegmentScreeningEvidence and their
// validators) stay in internal/filler, which is why this runtime cannot live there.
package fillerscreening
