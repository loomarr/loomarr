package playout

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// StartReason names why a tune produced no stream. The route turns it into the viewer's
// explanation, so the player can say what broke instead of holding "Tuning in" forever.
type StartReason string

const (
	// StartProgramSourceUnreachable: the session could not open its own programme endpoint
	// (dial or timeout). The encoder never received a block.
	StartProgramSourceUnreachable StartReason = "program_source_unreachable"
	// StartProgramSourceFailed: the programme endpoint answered but refused or malformed the block.
	StartProgramSourceFailed StartReason = "program_source_failed"
	// StartEncoderExited: ffmpeg died before writing its first segment.
	StartEncoderExited StartReason = "encoder_exited"
	// StartNoStream: nothing failed visibly, but no segment arrived within the start deadline.
	StartNoStream StartReason = "no_stream"
)

// StartError is the typed failure of a tune that never produced a first segment.
type StartError struct {
	Reason StartReason
	Err    error
}

func (e *StartError) Error() string { return e.Err.Error() }
func (e *StartError) Unwrap() error { return e.Err }

// classifyBlockOpen maps a failed block open onto the reason a viewer can act on. Prepared-only
// misses and cancellation are ordinary control flow, not faults, and report false.
func classifyBlockOpen(err error) (StartReason, bool) {
	if err == nil || errors.Is(err, ErrPreparedUnavailable) ||
		errors.Is(err, context.Canceled) {
		return "", false
	}
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
		return StartProgramSourceUnreachable, true
	}
	return StartProgramSourceFailed, true
}

// blockOpenAttemptsBeforeFailingTune is how many consecutive failed opens (250ms apart) a tune
// waiting for its first segment tolerates before it stops waiting out the start deadline: a
// programme source that refused this many times in a row is not warming up.
const blockOpenAttemptsBeforeFailingTune = 3

// noteBlockOpen records the latest block-open outcome so a tune still waiting for its first
// segment can report the real cause. A successful open clears the fault.
func (p *Process) noteBlockOpen(err error) {
	reason, fault := classifyBlockOpen(err)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err == nil {
		p.blockFault, p.blockFaults = nil, 0
		return
	}
	if fault {
		p.blockFault = &StartError{Reason: reason, Err: err}
		p.blockFaults++
	}
}

// blockOpenExhausted reports whether the programme source has failed enough consecutive opens
// that the tune should fail now rather than at the first-segment deadline.
func (p *Process) blockOpenExhausted() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.blockFaults >= blockOpenAttemptsBeforeFailingTune
}

// startFailure builds the typed error for a remux that yielded no stream. A recorded block-open
// fault outranks the generic reason: it is the cause, the missing segment only the symptom.
func (p *Process) startFailure(fallback StartReason, err error) error {
	var fault *StartError
	if p != nil {
		p.mu.Lock()
		fault = p.blockFault
		p.mu.Unlock()
	}
	if fault != nil {
		return &StartError{Reason: fault.Reason, Err: fmt.Errorf("%w (block source: %w)", err, fault.Err)}
	}
	return &StartError{Reason: fallback, Err: err}
}
