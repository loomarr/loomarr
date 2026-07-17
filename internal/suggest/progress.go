package suggest

import "context"

// Phase names a stage of grounded generation (§8), surfaced to the UI as SSE
// `suggestion` progress frames. A dropped frame is a latency bug, never a
// correctness bug — GET /v1/suggestions/{id} is the source of truth on reconnect.
type Phase string

const (
	PhaseSearching Phase = "searching" // grounding: the model queries the catalog tool
	PhaseReasoning Phase = "reasoning" // the model has composed its grounded lineup
	PhaseScoring   Phase = "scoring"   // deterministic post-scoring + proposal assembly
	// PhaseDone / PhaseFailed are emitted by the worker around Suggest, not from
	// inside it — the pipeline itself only knows the in-flight phases.
	PhaseDone   Phase = "done"
	PhaseFailed Phase = "failed"
)

// ProgressFunc receives each phase transition during Suggest. It must not block:
// the worker's publisher is non-blocking and drops on a full subscriber buffer.
type ProgressFunc func(Phase)

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

// reportProgress fires the context's ProgressFunc if one is set.
func reportProgress(ctx context.Context, p Phase) {
	if fn, ok := ctx.Value(progressKey{}).(ProgressFunc); ok && fn != nil {
		fn(p)
	}
}
