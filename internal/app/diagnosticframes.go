package app

import "time"

// Diagnostics frame pacing (#3352). The recorder stores a batch every flush interval while the
// server logs, and an encoder's output is snapshotted every flush interval while it runs; every
// admin with Diagnostics open would otherwise get a frame for each.
const (
	// diagnosticEventsFrameGap is the least time between diagnostic_events frames.
	diagnosticEventsFrameGap = 2 * time.Second
	// diagnosticProcessesFrameGap is the least time between diagnostic_processes frames while a
	// run writes output; a run that ends is announced at once.
	diagnosticProcessesFrameGap = time.Second
)

// diagnosticFrames publishes the Diagnostics pages' frames (eventEmitter).
type diagnosticFrames interface {
	DiagnosticEventsChanged()
	DiagnosticProcessesChanged()
}

// diagnosticFramesFor is the recorder's OnChange and the process manager's OnChange, each paced.
// frames nil publishes nothing.
func diagnosticFramesFor(frames diagnosticFrames) (eventsChanged func(), processesChanged func(finished bool)) {
	if frames == nil {
		return nil, nil
	}
	eventFrames := newFrameThrottle(diagnosticEventsFrameGap, frames.DiagnosticEventsChanged)
	processFrames := newFrameThrottle(diagnosticProcessesFrameGap, frames.DiagnosticProcessesChanged)
	return func() { eventFrames.poke(false) }, processFrames.poke
}
