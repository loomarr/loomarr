//go:build eval

package eval

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

func TestReleaseGateLatencyRequiresEveryTrialMeasurement(t *testing.T) {
	for name, results := range map[string][]Result{
		"empty suite":         nil,
		"missing measurement": {{EndToEndLatencyNanos: 1}, {}},
		"invalid measurement": {{EndToEndLatencyNanos: -1}},
	} {
		t.Run(name, func(t *testing.T) {
			assessment := assessCertification(results, CertificationThresholds{
				MaxP95EndToEndLatencyNanos: 12_000_000_000,
			}, ResourceMeasurement{})
			if assessment.Passed || !strings.Contains(strings.Join(assessment.Failures, ";"), "latency evidence is incomplete") {
				t.Fatalf("missing latency qualified: %+v", assessment)
			}
		})
	}
}

func TestReleaseGateLatencySeparatesFailedRequestsFromSuccessfulMaximum(t *testing.T) {
	assessment := assessCertification([]Result{
		{EndToEndLatencyNanos: 7_000_000_000},
		{EndToEndLatencyNanos: 30_000_000_000, Failures: []string{"provider failure"}},
	}, CertificationThresholds{
		MaxP95EndToEndLatencyNanos:        12_000_000_000,
		MaxSuccessfulEndToEndLatencyNanos: 20_000_000_000,
	}, ResourceMeasurement{})
	if assessment.Performance.MaxSuccessfulEndToEndLatencyNanos != 7_000_000_000 ||
		assessment.Performance.EndToEndLatencyP95Nanos != 30_000_000_000 ||
		assessment.Passed || len(assessment.Failures) != 1 {
		t.Fatalf("failed request distorted successful maximum or disappeared from p95: %+v", assessment)
	}
}

func TestReleaseGateAcceptableMembersCountDistinctIdentities(t *testing.T) {
	proposal := proposalWithKeys(t, "series:tmdb:20101")
	c := Case{AcceptableKeys: []provision.Key{"series:tmdb:20101", "series:tmdb:20101"}, MinAcceptableKeys: 2}
	if failures := deterministicChecks(c, proposal, nil); !strings.Contains(strings.Join(failures, ";"), "acceptable grounded members 1 < required 2") {
		t.Fatalf("duplicate identity supplied two members: %v", failures)
	}
}
