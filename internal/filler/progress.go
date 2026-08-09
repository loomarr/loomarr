package filler

import "context"

// Intra-stage progress reporting (§10 V51b) — a direct analogue of `internal/suggest/progress.go`,
// and deliberately the same shape so the two read alike at their SSE bridges.

// ProgressFunc receives progress WITHIN the running stage. It must not block: the publisher behind
// it is non-blocking and drops on a full subscriber buffer.
//
// `percent` is 0-100, or **-1 when the stage cannot measure itself**.
//
// ⚠ **-1 is a sentinel, not a small number, and callers must not render it as 0%.** Only the
// transcode stage has a real percentage — ffmpeg reports `out_time` and the clip's duration is
// already known, so the ratio is exact and free. Whisper and an LLM turn are single opaque calls:
// a bar interpolated over them would be invented progress, the failure the `database` frame's own
// comment names ("a UI that invented progress it had not been told about would be actively
// misleading"). A stage with no measurement says so, and the UI shows a spinner rather than a bar
// frozen at zero — the same distinction `confidence == 0` draws between "no measurement" and "a
// measurement of none".
type ProgressFunc func(stage StageID, percent int)

// NoMeasurement is the percent a stage reports when it genuinely cannot measure itself.
const NoMeasurement = -1

type progressKey struct{}

// WithProgress returns a context carrying fn.
//
// ⚠ Context-carried rather than a parameter, for the reason `suggest` gives: stages call deep into
// `Splitter.Propose`, `Classify`, `MediaTools.Transcribe` and the transcoder, several of which are
// shared with the API's on-demand paths. Threading a callback through all of them is a signature
// change across many call sites, and a bare context makes reporting a no-op — which is exactly
// what a unit test calling a stage directly wants.
func WithProgress(ctx context.Context, fn ProgressFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, fn)
}

// reportProgress fires the context's ProgressFunc if one is set.
func reportProgress(ctx context.Context, stage StageID, percent int) {
	if fn, ok := ctx.Value(progressKey{}).(ProgressFunc); ok && fn != nil {
		fn(stage, percent)
	}
}
