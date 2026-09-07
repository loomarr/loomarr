package playoutcert

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestSyntheticParentFaultRefusesWrongTargetAndStaleParent(t *testing.T) {
	target := &SyntheticTarget{BaseURL: "http://127.0.0.1:9999", parents: map[string]*syntheticParent{}}
	if _, err := target.CurrentParent(context.Background(), ParentFaultRequest{BaseURL: "http://127.0.0.1:9998", ChannelID: "channel"}); err == nil {
		t.Fatal("CurrentParent accepted a mismatched target")
	}
	if _, err := target.FailParent(context.Background(), ParentFaultRequest{BaseURL: target.BaseURL, ChannelID: "channel", Generation: 1}); err == nil {
		t.Fatal("FailParent accepted a stale parent")
	}
}

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

func TestHumanSummaryIncludesBoundedFaultQualifications(t *testing.T) {
	report := Report{
		FaultProfiles: []FaultQualification{
			{Profile: FaultChildFailure, Status: "unqualified", Outcome: "not_selected"},
			{Profile: FaultParentFailure, Status: "qualified", Outcome: "complete"},
			{Profile: FaultShutdown, Status: "unqualified", Outcome: "not_selected"},
		},
	}
	summary := HumanSummary(report)
	for _, want := range []string{
		"Fault profiles:",
		"child_failure status=unqualified outcome=not_selected",
		"parent_failure status=qualified outcome=complete",
		"shutdown status=unqualified outcome=not_selected",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
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
