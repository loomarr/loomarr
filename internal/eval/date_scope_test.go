//go:build eval

package eval

import (
	"context"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

func TestDateScopePolicyAccuracy(t *testing.T) {
	disjoint := &schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990, To: 1992}, {From: 2000, To: 2002}}}
	seriesAiring := &schedule.DateScope{SeriesAiring: []schedule.Range{{From: 1990, To: 1992}}}
	none := &schedule.DateScope{}
	for _, tc := range []struct {
		name     string
		expected *schedule.DateScope
		proposal suggest.Proposal
		err      error
		accurate bool
	}{
		{name: "exact disjoint windows", expected: disjoint, proposal: dateScopeProposal(disjoint, nil), accurate: true},
		{name: "convex hull widening", expected: disjoint, proposal: dateScopeProposal(&schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990, To: 2002}}}, nil)},
		{name: "missing dates", expected: disjoint, proposal: dateScopeProposal(nil, nil)},
		{name: "series premiere is not series airing", expected: seriesAiring, proposal: dateScopeProposal(&schedule.DateScope{SeriesPremiere: []schedule.Range{{From: 1990, To: 1992}}}, nil)},
		{name: "hidden scalar era", expected: disjoint, proposal: dateScopeProposal(disjoint, &schedule.Range{From: 1990, To: 2002})},
		{name: "explicit none accepts absence", expected: none, proposal: dateScopeProposal(nil, nil), accurate: true},
		{name: "explicit none rejects empty date scope", expected: none, proposal: dateScopeProposal(&schedule.DateScope{}, nil)},
		{name: "explicit none rejects dates", expected: none, proposal: dateScopeProposal(&schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990, To: 1992}}}, nil)},
		{name: "explicit none rejects scalar era", expected: none, proposal: dateScopeProposal(nil, &schedule.Range{From: 1990, To: 1992})},
		{name: "failed proposal cannot satisfy none", expected: none, err: suggest.ErrNoGroundedTitles},
	} {
		t.Run(tc.name, func(t *testing.T) {
			card := dateScopeRunner(tc.proposal, tc.err).Run(context.Background(), []Case{{
				Name: tc.name, NoFabrication: true, ExpectedDateScope: tc.expected,
			}})
			result := card.Results[0]
			if !result.PolicyAccuracyExpected || result.PolicyAccurate != tc.accurate {
				t.Fatalf("policy accuracy = expected:%v accurate:%v, want expected:true accurate:%v", result.PolicyAccuracyExpected, result.PolicyAccurate, tc.accurate)
			}
			if !result.Passed() || result.FailureStage != "" {
				t.Fatalf("date quality miss became a hard failure: %+v", result)
			}
			if card.Certified != tc.accurate {
				t.Fatalf("certification = %v, want %v: %+v", card.Certified, tc.accurate, card.Assessment)
			}
		})
	}
}

func TestDateScopeCeilingOnlyPolicyAccuracyRemainsUnchanged(t *testing.T) {
	proposal := dateScopeProposal(nil, nil)
	proposal.Policy.Audience.Ceiling = "TV-Y7"
	result := dateScopeRunner(proposal, nil).Run(context.Background(), []Case{{
		Name: "historical ceiling", NoFabrication: true, ExpectedPolicyCeiling: "TV-Y7",
	}}).Results[0]
	if !result.PolicyAccuracyExpected || !result.PolicyAccurate || !result.Passed() {
		t.Fatalf("ceiling-only policy scoring changed: %+v", result)
	}
}

func TestDateScopeResultOwnsCapturedPolicy(t *testing.T) {
	dates := &schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990, To: 1992}}}
	era := &schedule.Range{From: 1990, To: 1992}
	proposal := dateScopeProposal(dates, era)
	result := dateScopeRunner(proposal, nil).Run(context.Background(), []Case{{
		Name: "captured policy", NoFabrication: true, ExpectedDateScope: dates,
	}}).Results[0]
	dates.MovieRelease[0].From = 1900
	era.To = 1900
	wantDates := &schedule.DateScope{MovieRelease: []schedule.Range{{From: 1990, To: 1992}}}
	wantEra := &schedule.Range{From: 1990, To: 1992}
	if !reflect.DeepEqual(result.DateScope, wantDates) || !reflect.DeepEqual(result.ScalarEra, wantEra) {
		t.Fatalf("result policy changed after caller mutation: dates=%+v era=%+v", result.DateScope, result.ScalarEra)
	}
}

func dateScopeRunner(proposal suggest.Proposal, err error) *Runner {
	return NewRunner(scriptedGenerator{proposal: proposal, err: err}, RunnerConfig{Contract: &CertificationContract{
		Thresholds: CertificationThresholds{MinPolicyAccuracyRate: 1},
	}})
}

func dateScopeProposal(dates *schedule.DateScope, era *schedule.Range) suggest.Proposal {
	return suggest.Proposal{
		Lineup: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix"}},
		Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Scope: schedule.ScopePolicy{
			Dates: dates, Era: era,
		}}},
	}
}
