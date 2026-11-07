package storagegovernor

import "fmt"

// PauseError is the governor declining managed work: no lease, a failed revalidation, or a write
// that outgrew its lease. It is OPERATIONAL state, never a fact about the item being written
// (#2031), so callers that retry on failure must recognise it with errors.As and wait instead.
//
// ⚠ Every refusal the governor hands out goes through Pause. The filler pipeline once saw
// these as ordinary stage failures and, after its retries, rejected good clips as `unplayable`
// because the library allowance was full.
type PauseError struct {
	Operation string
	Reason    Reason
	Err       error
}

func (e *PauseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s (%s): %v", e.Operation, e.Reason, e.Err)
	}
	return fmt.Sprintf("%s (%s)", e.Operation, e.Reason)
}

func (e *PauseError) Unwrap() error { return e.Err }

// Pause reports a refused decision as a PauseError naming the operation that was turned away.
func Pause(operation string, decision Decision) error {
	return &PauseError{Operation: operation, Reason: decision.Snapshot.Reason, Err: decision.Err}
}
