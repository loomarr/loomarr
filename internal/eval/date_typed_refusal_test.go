//go:build eval

package eval

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

func TestV8TypedDateRefusalRunnerMatrix(t *testing.T) {
	for _, tc := range []struct {
		name      string
		expected  string
		proposal  suggest.Proposal
		err       error
		quality   bool
		schema    bool
		passed    bool
		certified bool
	}{
		{name: "conflict accepts conflict", expected: suggest.TerminalConstraintsConflict, err: typedDateFailure(suggest.TerminalConstraintsConflict), quality: true, schema: true, passed: true, certified: true},
		{name: "ambiguity accepts ambiguity", expected: suggest.TerminalDateSemanticsUnclear, err: typedDateFailure(suggest.TerminalDateSemanticsUnclear), quality: true, schema: true, passed: true, certified: true},
		{name: "conflict rejects ambiguity", expected: suggest.TerminalConstraintsConflict, err: typedDateFailure(suggest.TerminalDateSemanticsUnclear), schema: true, passed: true},
		{name: "ambiguity rejects conflict", expected: suggest.TerminalDateSemanticsUnclear, err: typedDateFailure(suggest.TerminalConstraintsConflict), schema: true, passed: true},
		{name: "wrapped conflict accepts conflict", expected: suggest.TerminalConstraintsConflict, err: fmt.Errorf("wrapped: %w", typedDateFailure(suggest.TerminalConstraintsConflict)), quality: true, schema: true, passed: true, certified: true},
		{name: "generic no grounded titles is not typed quality", expected: suggest.TerminalConstraintsConflict, err: suggest.ErrNoGroundedTitles, schema: true, passed: true},
		{name: "successful empty proposal is not typed quality", expected: suggest.TerminalConstraintsConflict, schema: true, passed: true},
		{name: "malformed terminal is not typed", expected: suggest.TerminalConstraintsConflict, err: typedDateFailure(suggest.TerminalMalformedExhausted), passed: false},
		{name: "wrong failure code is not typed", expected: suggest.TerminalConstraintsConflict, err: suggest.NewFailure(suggest.FailureProvider, validDateTrace(suggest.TerminalConstraintsConflict), errors.New("provider failed")), passed: false},
		{name: "invalid trace version is not typed", expected: suggest.TerminalConstraintsConflict, err: suggest.NewFailure(suggest.FailureCodeNoGroundedTitles, suggest.DecisionTrace{Version: suggest.DecisionTraceVersion + 1, Terminal: suggest.TerminalConstraintsConflict}, errors.New("date refusal")), passed: false},
		{name: "typed refusal with picks is not typed", expected: suggest.TerminalConstraintsConflict, proposal: suggest.Proposal{Lineup: []suggest.ProposalItem{{Name: "unexpected pick"}}}, err: typedDateFailure(suggest.TerminalConstraintsConflict), passed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			card := typedDateRefusalRunner(tc.proposal, tc.err).Run(context.Background(), []Case{{
				Name: tc.name, NoFabrication: true, ExpectedProposalTerminal: tc.expected,
			}})
			result := card.Results[0]
			if !result.ProposalQualityExpected || result.ProposalQuality != tc.quality || result.SchemaValid != tc.schema || result.Passed() != tc.passed || card.Certified != tc.certified {
				t.Fatalf("result/certification = qualityExpected:%v quality:%v schema:%v passed:%v certified:%v, want true/%v/%v/%v/%v: %+v", result.ProposalQualityExpected, result.ProposalQuality, result.SchemaValid, result.Passed(), card.Certified, tc.quality, tc.schema, tc.passed, tc.certified, result)
			}
			if card.Assessment.ProposalQualityRate != boolRate(tc.quality) {
				t.Fatalf("proposal quality rate = %v, want %v: %+v", card.Assessment.ProposalQualityRate, boolRate(tc.quality), card.Assessment)
			}
		})
	}
}

func TestV8TypedDateRefusalKeepsAbstentionAndNoFabricationContracts(t *testing.T) {
	for _, tc := range []struct {
		name       string
		proposal   suggest.Proposal
		err        error
		quality    bool
		schema     bool
		passed     bool
		noTypedRef bool
	}{
		{name: "old abstention accepts ordinary no grounded titles", err: suggest.ErrNoGroundedTitles, quality: true, schema: true, passed: true},
		{name: "old abstention rejects typed ordinary cause", err: typedDateFailure(suggest.TerminalConstraintsConflict), schema: false, passed: false},
		{name: "old abstention rejects successful empty proposal", quality: false, schema: true, passed: true},
		{name: "old abstention rejects nonempty proposal", proposal: suggest.Proposal{Lineup: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"}}}, quality: false, schema: true, passed: true},
		{name: "no typed expectation preserves rejection of typed ordinary cause", err: typedDateFailure(suggest.TerminalConstraintsConflict), quality: true, schema: false, passed: false, noTypedRef: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Case{Name: tc.name, NoFabrication: true}
			if !tc.noTypedRef {
				c.ExpectedProposalAbstention = true
			}
			result := typedDateRefusalRunner(tc.proposal, tc.err).Run(context.Background(), []Case{c}).Results[0]
			if result.ProposalQuality != tc.quality || result.SchemaValid != tc.schema || result.Passed() != tc.passed {
				t.Fatalf("result = quality:%v schema:%v passed:%v, want %v/%v/%v: %+v", result.ProposalQuality, result.SchemaValid, result.Passed(), tc.quality, tc.schema, tc.passed, result)
			}
			if tc.noTypedRef && result.ProposalQualityExpected {
				t.Fatalf("unrequested typed refusal unexpectedly evaluated proposal quality: %+v", result)
			}
		})
	}
}

func TestV8TypedDateRefusalRetainsGroundingAndDateAccuracyInvariants(t *testing.T) {
	t.Run("minimum grounded still fails", func(t *testing.T) {
		result := typedDateRefusalRunner(suggest.Proposal{}, typedDateFailure(suggest.TerminalConstraintsConflict)).Run(context.Background(), []Case{{
			Name: "minimum grounded", NoFabrication: true, MinGrounded: 1, ExpectedProposalTerminal: suggest.TerminalConstraintsConflict,
		}}).Results[0]
		if result.Passed() || !result.SchemaValid || !result.ProposalQuality {
			t.Fatalf("typed refusal bypassed minimum grounding: %+v", result)
		}
	})

	t.Run("failed proposal cannot satisfy explicit no dates", func(t *testing.T) {
		card := NewRunner(scriptedGenerator{err: typedDateFailure(suggest.TerminalConstraintsConflict)}, RunnerConfig{Contract: &CertificationContract{
			Thresholds: CertificationThresholds{MinPolicyAccuracyRate: 1},
		}}).Run(context.Background(), []Case{{
			Name: "failed none", NoFabrication: true, ExpectedProposalTerminal: suggest.TerminalConstraintsConflict, ExpectedDateScope: &schedule.DateScope{},
		}})
		result := card.Results[0]
		if result.PolicyAccurate || !result.SchemaValid || !result.ProposalQuality || !result.Passed() || card.Certified {
			t.Fatalf("failed typed proposal satisfied date none: result=%+v assessment=%+v", result, card.Assessment)
		}
	})
}

func typedDateRefusalRunner(proposal suggest.Proposal, err error) *Runner {
	return NewRunner(scriptedGenerator{proposal: proposal, err: err}, RunnerConfig{Contract: &CertificationContract{
		Thresholds: CertificationThresholds{MinProposalQualityRate: 1, MinSchemaValidityRate: 1},
	}})
}

func TestV8TypedDateRefusalRequiresNoCatalogActivity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		toolCalls  int
		dispatches int
		certified  bool
	}{
		{name: "pre-dispatch refusal", certified: true},
		{name: "observed tool operation", toolCalls: 1},
		{name: "source dispatch in failure trace", dispatches: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trace := validDateTrace(suggest.TerminalConstraintsConflict)
			trace.SourceQueriesDispatched = tc.dispatches
			err := suggest.NewFailure(suggest.FailureCodeNoGroundedTitles, trace, errors.New("date conflict"))
			card := NewRunner(scriptedGenerator{err: err}, RunnerConfig{Contract: &CertificationContract{
				Thresholds: CertificationThresholds{MinProposalQualityRate: 1, MinCorrectToolOperationRate: 1},
			}}).WithObserver(&scriptedObserver{value: Observation{ToolCalls: tc.toolCalls}}).Run(context.Background(), []Case{{
				Name: tc.name, NoFabrication: true, ExpectedProposalTerminal: suggest.TerminalConstraintsConflict, ExpectedToolOperation: "none",
			}})
			if card.Certified != tc.certified || !card.Results[0].Passed() {
				t.Fatalf("certified=%v want=%v assessment=%+v failures=%v", card.Certified, tc.certified, card.Assessment, card.Results[0].Failures)
			}
		})
	}
}

func boolRate(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func validDateTrace(terminal string) suggest.DecisionTrace {
	return suggest.DecisionTrace{Version: suggest.DecisionTraceVersion, Terminal: terminal}
}

func typedDateFailure(terminal string) error {
	return suggest.NewFailure(suggest.FailureCodeNoGroundedTitles, validDateTrace(terminal), errors.New("date refusal"))
}
