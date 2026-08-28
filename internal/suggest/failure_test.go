package suggest_test

import (
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/suggest"
)

func TestFailureTraceJSONFailsClosedAndCopiesCandidates(t *testing.T) {
	trace := suggest.DecisionTrace{Version: suggest.DecisionTraceVersion, SurfacedTotal: 1, RecordedTotal: 1,
		Candidates: []suggest.DecisionCandidate{{Key: "movie:tmdb:1", Ownership: "library", Disposition: suggest.DispositionSelected, Reason: "selected"}}}
	err := suggest.NewFailure("provider_failure", trace, errors.New("provider detail"))
	trace.Candidates[0].Key = "mutated"
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Trace.Candidates[0].Key != "movie:tmdb:1" {
		t.Fatalf("failure did not copy original trace: %+v", failure)
	}
	if _, err := failure.TraceJSON(); err != nil {
		t.Fatalf("valid trace rejected: %v", err)
	}
	for name, bad := range map[string]suggest.DecisionTrace{
		"version":  {Version: 2},
		"bounds":   {Version: 1, SurfacedTotal: 65, RecordedTotal: 65, Truncated: false},
		"totals":   {Version: 1, SurfacedTotal: 1, RecordedTotal: -1},
		"reason":   {Version: 1, SurfacedTotal: 1, RecordedTotal: 1, Candidates: []suggest.DecisionCandidate{{Disposition: suggest.DispositionSelected, Reason: suggest.ReasonNever}}},
		"terminal": {Version: 1, Terminal: "provider-secret"},
	} {
		badErr := suggest.NewFailure("provider_failure", bad, nil)
		var badFailure *suggest.Failure
		if !errors.As(badErr, &badFailure) {
			t.Fatalf("%s did not expose typed failure", name)
		}
		if _, err := badFailure.TraceJSON(); err == nil {
			t.Errorf("%s trace unexpectedly serialized", name)
		}
	}
}
