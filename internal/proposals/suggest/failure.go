package suggest

import (
	"encoding/json"
	"fmt"

	"github.com/loomarr/loomarr/internal/proposals"
)

// Failure is the public, safe failure seam for one suggestion run. Cause is
// available only for errors.Is/As inside the worker; Code and Trace are the
// only facts suitable for persistence or Journey projection.
type Failure struct {
	Code  string
	Trace proposals.DecisionTrace
	Cause error
}

func (f *Failure) Error() string {
	message := map[string]string{
		FailureSelectionEmpty:       "no grounded titles",
		FailureCodeNoGroundedTitles: "no grounded titles",
		FailureBudgetExhausted:      "suggestion budget exhausted",
		FailureProvider:             "provider failure",
	}[f.Code]
	if message == "" {
		message = "suggestion failure"
	}
	return fmt.Sprintf("suggestion failed: %s", message)
}
func (f *Failure) Unwrap() error { return f.Cause }

func (f *Failure) TraceJSON() (string, error) {
	if err := proposals.ValidateDecisionTrace(f.Trace); err != nil {
		return "", fmt.Errorf("invalid suggestion failure trace: %w", err)
	}
	blob, err := json.Marshal(f.Trace)
	if err != nil {
		return "", fmt.Errorf("marshal bounded suggestion failure trace: %w", err)
	}
	return string(blob), nil
}

func NewFailure(code string, trace proposals.DecisionTrace, cause error) error {
	trace = trace.Clone()
	return &Failure{Code: code, Trace: trace, Cause: cause}
}

const (
	FailureSelectionEmpty  = "selection_empty"
	FailureBudgetExhausted = "budget_exhausted"
	FailureProvider        = "provider_failure"
)
