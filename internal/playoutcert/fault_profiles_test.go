package playoutcert

import (
	"context"
	"strings"
	"sync"
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

func TestSyntheticShutdownRefusesMismatchAndSharesOneTerminalReceipt(t *testing.T) {
	target := &SyntheticTarget{BaseURL: "http://127.0.0.1:9999", scope: "isolated"}
	if _, err := target.Shutdown(context.Background(), ShutdownRequest{BaseURL: "http://127.0.0.1:9998"}); err == nil {
		t.Fatal("Shutdown accepted a mismatched target")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := target.Shutdown(cancelled, ShutdownRequest{BaseURL: target.BaseURL}); err == nil {
		t.Fatal("Shutdown reported success for a cancelled caller")
	}
	var wg sync.WaitGroup
	receipts := make([]ShutdownReceipt, 2)
	errs := make([]error, 2)
	for index := range receipts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipts[index], errs[index] = target.Shutdown(context.Background(), ShutdownRequest{BaseURL: target.BaseURL})
		}()
	}
	wg.Wait()
	for index := range receipts {
		if errs[index] != nil || receipts[index].Scope != target.scope || !receipts[index].ServingStopped || !receipts[index].ProcessesExited {
			t.Fatalf("shutdown receipt %d = %+v, %v", index, receipts[index], errs[index])
		}
	}
	if _, err := target.SampleStopped(context.Background(), "final"); err == nil {
		t.Fatal("SampleStopped fabricated a sample without an owned retained projection")
	}
}

func TestSyntheticCloseExecutesShutdownButDoesNotQualifyDrill(t *testing.T) {
	target := &SyntheticTarget{BaseURL: "http://127.0.0.1:9999", scope: "isolated"}
	if err := target.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if receipt, err := target.Shutdown(context.Background(), ShutdownRequest{BaseURL: target.BaseURL}); err != nil || !receipt.ServingStopped || !receipt.ProcessesExited {
		t.Fatalf("Close did not preserve the terminal stop: receipt=%+v err=%v", receipt, err)
	}
	rows, failures := faultQualifications([]FaultProfile{FaultShutdown}, target)
	if len(failures) != 1 || rows[2].Status != "unavailable" || rows[2].Outcome != "controller_not_exercised" {
		t.Fatalf("ordinary Close qualified shutdown: rows=%+v failures=%v", rows, failures)
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
