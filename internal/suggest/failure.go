package suggest

import (
	"encoding/json"
	"fmt"
)

// Failure is the public, safe failure seam for one suggestion run. Cause is
// available only for errors.Is/As inside the worker; Code and Trace are the
// only facts suitable for persistence or Journey projection.
type Failure struct {
	Code  string
	Trace DecisionTrace
	Cause error
}

func (f *Failure) Error() string { return fmt.Sprintf("suggestion failed: %s", f.Code) }
func (f *Failure) Unwrap() error { return f.Cause }

func (f *Failure) TraceJSON() (string, error) {
	blob, err := json.Marshal(f.Trace)
	if err != nil {
		return "", fmt.Errorf("marshal bounded suggestion failure trace: %w", err)
	}
	return string(blob), nil
}

func NewFailure(code string, trace DecisionTrace, cause error) error {
	trace.Candidates = append([]DecisionCandidate(nil), trace.Candidates...)
	return &Failure{Code: code, Trace: trace, Cause: cause}
}

const (
	FailureSelectionEmpty  = "selection_empty"
	FailureBudgetExhausted = "budget_exhausted"
	FailureProvider        = "provider_failure"
)
