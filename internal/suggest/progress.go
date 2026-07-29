package suggest

import "context"

// Phase names a stage of grounded generation (§8), surfaced to the UI as SSE
// `suggestion` progress frames. A dropped frame is a latency bug, never a
// correctness bug — GET /v1/suggestions/{id} is the source of truth on reconnect.
type Phase string

const (
	// PhaseSearching: a catalog tool call is RUNNING (the model asked for titles and
	// we're fetching them). Short — it's a local/TMDB query, not a model turn.
	PhaseSearching Phase = "searching"
	// PhaseReasoning: the model is THINKING — the turn is in flight and we're waiting
	// on it. This is where a slow run actually spends its time (a cold local model can
	// take ~9s just to load before it emits a token), which is precisely why it must be
	// reported while it happens rather than after.
	PhaseReasoning Phase = "reasoning"
	// PhaseScoring: deterministic post-scoring + proposal assembly. Outside the loop.
	PhaseScoring Phase = "scoring"
	// PhaseDone / PhaseFailed are emitted by the worker around Suggest, not from
	// inside it — the pipeline itself only knows the in-flight phases.
	PhaseDone   Phase = "done"
	PhaseFailed Phase = "failed"
)

// ProgressFunc receives each phase transition during Suggest. It must not block:
// the worker's publisher is non-blocking and drops on a full subscriber buffer.
//
// round is the 1-based tool-loop iteration the phase belongs to, or 0 for a phase
// outside the loop (scoring, and the worker's done/failed). Phases REPEAT: the loop
// alternates reasoning → searching → reasoning as the model calls the catalog and
// reads the results, so a receiver must treat these as "what is happening now",
// not as a monotonic sequence of distinct steps.
type ProgressFunc func(Phase, int)

type progressKey struct{}

// WithProgress returns a context carrying fn so Suggest can report phase
// transitions without a signature change across its many call sites. The worker
// injects this per job so the callback can tag frames with the job id; a bare
// context (unit tests calling Suggest directly) makes reporting a no-op.
func WithProgress(ctx context.Context, fn ProgressFunc) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, fn)
}

// reportProgress fires the context's ProgressFunc if one is set, tagging the frame
// with the tool-loop round (0 outside the loop).
func reportProgress(ctx context.Context, p Phase, round int) {
	if fn, ok := ctx.Value(progressKey{}).(ProgressFunc); ok && fn != nil {
		fn(p, round)
	}
}
