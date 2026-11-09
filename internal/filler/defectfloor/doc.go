// Package defectfloor is the filler defect floor (#2034, #2036): it measures every
// published ad copy for picture and sound defects against an era-relative bar (the pure checks live
// in internal/media/defects), asks a vision model to rule on the borderline ones within a spend ceiling,
// records each verdict, and retires a copy that definitively fails once its fingerprint is saved.
// It also holds the DEFECTS conveyor stage that enforces those verdicts at admission, and the
// household report that scores the floor's bars against the maintainer's own labels.
//
// It sits above internal/filler and imports it for the stage contract, and cliplibrary, asset and
// pipelinestate for the clip catalog types, sidecar tags and pipeline state; internal/filler knows
// nothing of it. It cannot live in internal/media/defects, which internal/filler (through
// internal/media/tools) already imports.
package defectfloor
