package playoutcert

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestFaultProfileSelectionFailsClosed(t *testing.T) {
	valid := func() Config {
		return Config{
			BaseURL: "http://127.0.0.1:8080", AdminBearer: "admin", DeviceToken: "device",
			Channels: fixtureChannels(100), Certify: true,
			FaultProfiles: []FaultProfile{FaultChildFailure}, FaultController: playoutcertfixture.ScopedFaultController{ScopeName: "run-a"},
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"unknown", func(c *Config) { c.FaultProfiles = []FaultProfile{"other"} }, "unknown"},
		{"duplicate", func(c *Config) { c.FaultProfiles = []FaultProfile{FaultChildFailure, FaultChildFailure} }, "duplicate"},
		{"conflicting terminal faults", func(c *Config) { c.FaultProfiles = []FaultProfile{FaultParentFailure, FaultShutdown} }, "shutdown must be selected alone"},
		{"shutdown missing acknowledgement", func(c *Config) { c.FaultProfiles = []FaultProfile{FaultShutdown} }, "disposable"},
		{"shutdown wrong target", func(c *Config) { c.FaultProfiles = []FaultProfile{FaultShutdown}; c.DisposableTarget = "run-b" }, "match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := valid()
			tc.mutate(&config)
			if err := config.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestHumanSummaryForUnfinalizedReportDoesNotLeakFaultQualifications(t *testing.T) {
	report := Report{
		FaultProfiles: []FaultQualification{
			{Profile: FaultChildFailure, Status: "unqualified", Outcome: "not_selected"},
			{Profile: FaultParentFailure, Status: "qualified", Outcome: "complete"},
			{Profile: FaultShutdown, Status: "unqualified", Outcome: "not_selected"},
		},
	}
	summary := HumanSummary(report)
	if summary != "Playout load certification: FAIL\nAudit: missing (publication_not_finalized)\n" {
		t.Fatalf("unfinalized summary = %q", summary)
	}
	for _, leaked := range []string{"Fault profiles:", "parent_failure", "qualified"} {
		if strings.Contains(summary, leaked) {
			t.Fatalf("unfinalized summary leaked %q: %s", leaked, summary)
		}
	}
}

func TestMissingRequiredFaultResourceInvalidatesQualification(t *testing.T) {
	report := Report{FaultProfiles: []FaultQualification{{Profile: FaultParentFailure, Status: "qualified", Outcome: "complete"}}}
	invalidateQualifiedFaults(&report, "final_resource_sample_failed")
	row := report.FaultProfiles[0]
	if row.Status != "unavailable" || row.Outcome != "final_resource_sample_failed" {
		t.Fatalf("fault qualification after missing resource = %+v", row)
	}
}

func TestParentFaultCleanupResidualRetainsFinalEvidenceButDisqualifiesOutcome(t *testing.T) {
	sample := ResourceSample{Point: "final", Capacity: 1}
	for _, profile := range []FaultProfile{FaultParentFailure, FaultChildFailure} {
		report := Report{
			FaultProfiles: []FaultQualification{{Profile: profile, Status: "qualified", Outcome: "complete", Final: &sample, ReceiptOutcome: "exited", SelectedContinuity: "interrupted", Recovery: "recovered"}},
			Phases: []Phase{
				{Name: "parent_failure", LatencySummary: LatencySummary{Attempts: 1, Successes: 1}, Resources: PhaseResources{Samples: 1}},
				{Name: "child_failure", LatencySummary: LatencySummary{Attempts: 1, Successes: 1}, Resources: PhaseResources{Samples: 1}},
				{Name: "capacity_recovery", LatencySummary: LatencySummary{Attempts: 1, Successes: 1}, Resources: PhaseResources{Samples: 1}},
				{Name: "cleanup", LatencySummary: LatencySummary{Attempts: 1, Failures: 1}, Resources: PhaseResources{Samples: 1}},
			},
		}
		invalidateFaultsWithoutConvergence(&report)
		row := report.FaultProfiles[0]
		if row.Status != "unavailable" || row.Outcome != "cleanup_failed" || row.Final == nil || row.Final.Capacity != 1 {
			t.Fatalf("%s cleanup residual qualification = %+v", profile, row)
		}
	}
}
