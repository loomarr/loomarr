// Package fillerscreening is the rendered-child screening runtime: the four-axis coordinator that
// records one closed, replayable result per axis for an immutable child subject, the private
// content-addressed evidence repository behind it, the playback-integrity and qualification
// evaluators, the release certification that replays an aggregate against its authority, and the
// SCREEN conveyor stage that turns the outcome into a pipeline verdict. It owns the four-axis
// aggregate (SegmentScreeningEvidence) and the per-axis result model it is built from.
//
// It sits above internal/filler and imports it for the screening subject and the sidecar
// reference, the clip catalog types and the stage contract; internal/filler knows nothing of it.
// The sidecar embeds only the subject and the digest-only SegmentScreeningReference, so those stay
// in internal/filler; the aggregate does not enter the sidecar, so it lives here.
package fillerscreening
