package suggest_test

import (
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/suggest"
)

func TestFailureTraceJSONFailsClosedAndCopiesCandidates(t *testing.T) {
	trace := suggest.DecisionTrace{Version: suggest.DecisionTraceVersion, SurfacedTotal: 1, RecordedTotal: 1,
		Candidates: []suggest.DecisionCandidate{{Key: "movie:tmdb:1", Ownership: "library", Rank: suggest.RankTuple{TieKey: "movie:tmdb:1"}, Disposition: suggest.DispositionSelected, Reason: "selected"}}}
	err := suggest.NewFailure("provider_failure", trace, errors.New("provider detail"))
	trace.Candidates[0].Key = "mutated"
	var failure *suggest.Failure
	if !errors.As(err, &failure) || failure.Trace.Candidates[0].Key != "movie:tmdb:1" {
		t.Fatalf("failure did not copy original trace: %+v", failure)
	}
	if _, err := failure.TraceJSON(); err != nil {
		t.Fatalf("valid trace rejected: %v", err)
	}
	malformed := suggest.NewFailure("selection_empty", suggest.DecisionTrace{Version: 1, SurfacedTotal: 1, RecordedTotal: 2, Candidates: []suggest.DecisionCandidate{{Disposition: suggest.DispositionValidationDropped, Reason: suggest.ReasonMalformedID}}}, nil)
	var malformedFailure *suggest.Failure
	if !errors.As(malformed, &malformedFailure) {
		t.Fatal("malformed outcome lost typed failure")
	}
	if _, err := malformedFailure.TraceJSON(); err != nil {
		t.Fatalf("keyless malformed evidence rejected: %v", err)
	}
	for name, trace := range map[string]suggest.DecisionTrace{
		"keyed malformed evidence":     {Version: 1, SurfacedTotal: 1, RecordedTotal: 1, Candidates: []suggest.DecisionCandidate{{Key: "movie:tmdb:1", Disposition: suggest.DispositionValidationDropped, Reason: suggest.ReasonMalformedID}}},
		"keyful not-surfaced evidence": {Version: 1, SurfacedTotal: 1, RecordedTotal: 1, Candidates: []suggest.DecisionCandidate{{Key: "movie:tmdb:1", Disposition: suggest.DispositionValidationDropped, Reason: suggest.ReasonNotSurfaced}}},
	} {
		failureErr := suggest.NewFailure("selection_empty", trace, nil)
		var traceFailure *suggest.Failure
		if !errors.As(failureErr, &traceFailure) {
			t.Fatal("trace outcome lost typed failure")
		}
		serializedErr := func() error {
			_, err := traceFailure.TraceJSON()
			return err
		}()
		if name == "keyed malformed evidence" && serializedErr == nil {
			t.Fatal("keyed malformed evidence unexpectedly serialized")
		}
		if name == "keyful not-surfaced evidence" && serializedErr != nil {
			t.Fatalf("keyful not-surfaced evidence rejected: %v", serializedErr)
		}
	}
	for name, bad := range map[string]suggest.DecisionTrace{
		"version":     {Version: 2},
		"bounds":      {Version: 1, SurfacedTotal: 65, RecordedTotal: 65, Truncated: false},
		"totals":      {Version: 1, SurfacedTotal: 1, RecordedTotal: -1},
		"reason":      {Version: 1, SurfacedTotal: 1, RecordedTotal: 1, Candidates: []suggest.DecisionCandidate{{Disposition: suggest.DispositionSelected, Reason: suggest.ReasonNever}}},
		"terminal":    {Version: 1, Terminal: "provider-secret"},
		"total-bound": {Version: 1, SurfacedTotal: suggest.DecisionTraceMaxTotal + 1, RecordedTotal: suggest.DecisionTraceMaxTotal + 1, Truncated: true},
		"rank":        {Version: 1, SurfacedTotal: 1, RecordedTotal: 1, Candidates: []suggest.DecisionCandidate{{Key: "movie:tmdb:1", Ownership: "library", Rank: suggest.RankTuple{TieKey: "movie:tmdb:1", Preference: 4}, Disposition: suggest.DispositionSelected, Reason: "selected"}}},
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
