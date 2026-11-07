// Package screening is the rendered-child screening runtime: the four-axis coordinator that
// records one closed, replayable result per axis for an immutable child subject, the private
// content-addressed evidence repository behind it, the playback-integrity and qualification
// evaluators, the release certification that replays an aggregate against its authority, and the
// SCREEN conveyor stage that turns the outcome into a pipeline verdict. It owns the four-axis
// aggregate (SegmentScreeningEvidence), the per-axis result model it is built from, and
// Airworthiness: the deterministic audience policy that reduces the three safety axes' certified
// observations to the one decision every aggregate carries. It also owns the spoken-safety
// cascade: the fail-closed complete-source shadow reduction and the closed contract of its
// append-only execution ledger, which internal/filler/records persists.
//
// It sits above internal/filler and imports it for the screening subject and the sidecar
// reference, the clip catalog types and the stage contract; internal/filler knows nothing of it.
// The sidecar embeds only the subject and the digest-only SegmentScreeningReference, so those stay
// in internal/filler; the aggregate does not enter the sidecar, so it lives here.
package screening
