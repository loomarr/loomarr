package playoutcert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func requireUncertifiedPublication(t *testing.T, report Report) {
	t.Helper()
	if report.Certified || report.AuditStatus != AuditMissing {
		t.Fatalf("Run report certification boundary = certified=%t audit=%q, want false/missing", report.Certified, report.AuditStatus)
	}
	publication, _ := FinalizePublication(report)
	if publication.Verdict() == VerdictCertified || publication.ExitStatus() == 0 {
		t.Fatalf("failed run publication = audit=%q verdict=%q exit=%d, want non-certified/nonzero", publication.AuditStatus(), publication.Verdict(), publication.ExitStatus())
	}
}

func requireCertifiedPublication(t *testing.T, report Report) {
	t.Helper()
	if report.Certified || report.AuditStatus != AuditMissing {
		t.Fatalf("Run report certification boundary = certified=%t audit=%q, want false/missing", report.Certified, report.AuditStatus)
	}
	publication, err := FinalizePublication(report)
	if err != nil || publication.AuditStatus() != AuditPassed || publication.Verdict() != VerdictCertified || publication.ExitStatus() != 0 {
		t.Fatalf("publication = audit=%q verdict=%q exit=%d err=%v, want passed/certified/zero", publication.AuditStatus(), publication.Verdict(), publication.ExitStatus(), err)
	}
}

func TestValidateConfigRejectsUnsafeOrNonCertifyingInputs(t *testing.T) {
	channels := fixtureChannels(100)
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "credential in origin", mutate: func(c *Config) { c.BaseURL += "?token=secret" }, want: "query"},
		{name: "too few channels", mutate: func(c *Config) { c.Channels = c.Channels[:99] }, want: "at least 100"},
		{name: "duplicate channel", mutate: func(c *Config) { c.Channels[99].ID = c.Channels[0].ID }, want: "duplicate"},
		{name: "remote without acknowledgement", mutate: func(c *Config) { c.BaseURL = "http://192.0.2.1:8080" }, want: "remote"},
		{name: "missing bearer", mutate: func(c *Config) { c.AdminBearer = "" }, want: "administrator"},
		{name: "missing device token", mutate: func(c *Config) { c.DeviceToken = "" }, want: "device"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{BaseURL: "http://127.0.0.1:8080", AdminBearer: "admin-secret", DeviceToken: "device-secret", Channels: append([]Channel(nil), channels...), Certify: true}
			tc.mutate(&cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestShutdownFinalizationRejectsGlobalResourceFailures(t *testing.T) {
	baseline := ResourceSample{Point: "baseline", Capacity: 1}
	tests := []struct {
		name       string
		resources  []ResourceSample
		final      ResourceSample
		preFailure string
		want       string
	}{
		{name: "capacity oversubscribed", resources: []ResourceSample{baseline, {Point: "raw_capacity", Capacity: 1, TranscodeCost: 2}}, final: ResourceSample{Point: "shutdown_final", Capacity: 1}, want: "capacity_oversubscribed"},
		{name: "missing final resource", resources: []ResourceSample{baseline}, preFailure: "final_resource_sample_failed", want: "final_resource_sample_failed"},
		{name: "residual final resource", resources: []ResourceSample{baseline}, final: ResourceSample{Point: "shutdown_final", Capacity: 1, SessionsActive: 1}, want: "cleanup_residual"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := Report{Resources: tc.resources, Failures: []string{}, FaultProfiles: []FaultQualification{{Profile: FaultShutdown, Status: "qualified", Outcome: "complete"}}}
			if tc.preFailure != "" {
				report.Failures = append(report.Failures, tc.preFailure)
			}
			finalizeReport(&report, Config{PreparedP95: time.Second, PreparedRawP95: time.Second}, baseline, Phase{PreparedHits: 1}, Phase{}, 1, tc.final)
			if !slices.Contains(report.Failures, tc.want) {
				t.Fatalf("failures = %v, want %q", report.Failures, tc.want)
			}
			invalidateQualifiedFaults(&report, "run_failed")
			row, ok := faultQualification(&report, FaultShutdown)
			if !ok || row.Status != "unavailable" || row.Outcome != "run_failed" {
				t.Fatalf("shutdown fault qualification = %+v", row)
			}
		})
	}
}

func TestValidateConfigAppliesAndEnforcesProgrammeBoundaryBounds(t *testing.T) {
	valid := func() Config {
		return Config{BaseURL: "http://127.0.0.1:8080", AdminBearer: "admin-secret", DeviceToken: "device-secret", Channels: fixtureChannels(100), Certify: true}
	}
	defaults := valid()
	if err := defaults.Validate(); err != nil {
		t.Fatalf("zero boundary fields rejected: %v", err)
	}
	defaults = defaults.normalized()
	if defaults.ProgrammeBoundaryTimeout != 20*time.Minute || defaults.ProgrammeBoundaryLateObservation != 3*time.Second {
		t.Fatalf("documented defaults = timeout %s late %s", defaults.ProgrammeBoundaryTimeout, defaults.ProgrammeBoundaryLateObservation)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "negative timeout", mutate: func(c *Config) { c.ProgrammeBoundaryTimeout = -time.Second }},
		{name: "timeout below floor", mutate: func(c *Config) { c.ProgrammeBoundaryTimeout = time.Second }},
		{name: "timeout above ceiling", mutate: func(c *Config) { c.ProgrammeBoundaryTimeout = 25*time.Minute + time.Nanosecond }},
		{name: "late below floor", mutate: func(c *Config) { c.ProgrammeBoundaryLateObservation = 249 * time.Millisecond }},
		{name: "late above ceiling", mutate: func(c *Config) { c.ProgrammeBoundaryLateObservation = 30*time.Second + time.Nanosecond }},
		{name: "late equals timeout", mutate: func(c *Config) {
			c.ProgrammeBoundaryTimeout, c.ProgrammeBoundaryLateObservation = 2*time.Second, 2*time.Second
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := valid()
			tc.mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("invalid programme boundary bounds were accepted")
			}
		})
	}
}

func TestRunExercisesPublicPhasesButCannotCertifyWithoutCausalBoundaryEvidence(t *testing.T) {
	t.Parallel()
	fixture := playoutcertfixture.New(t, 100)
	validator := &recordingValidator{}
	decoder := &playoutcertfixture.Decoder{}
	cfg := Config{
		BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, DeviceToken: fixture.Device,
		Channels: fixtureChannels(100), Certify: true, RemoteAcknowledged: true,
		Concurrency: 12, SurfRounds: 1, FanInViewers: 4, RequestTimeout: time.Second,
		CleanupTimeout: time.Second, CleanupPoll: time.Millisecond, RawCaptureBytes: 188,
		Validator: validator, Decoder: decoder,
	}
	report, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(report.Failures, ","), "programme_boundary_failed") {
		t.Fatalf("ordinary fixture omitted causal boundary failure: %+v", report.Failures)
	}
	boundary := report.PhaseMust("programme_boundary")
	// The fixture has no independent programme truth. The prepared lane fails
	// before a media observation; the absent cold cohort remains separately missing.
	if boundary.HTTPClasses["evidence_unavailable"] != 1 || boundary.HTTPClasses["cohort_missing"] != 1 {
		t.Fatalf("programme boundary evidence limit = %+v", boundary)
	}
	if report.Target.ConfiguredChannels != 100 || report.Target.Capacity != 4 {
		t.Fatalf("target = %+v", report.Target)
	}
	if report.Resources[0].PreparedChannels != 100 || report.Resources[0].ReadyChannels != 100 {
		t.Fatalf("baseline prepared readiness = %+v", report.Resources[0])
	}
	for _, name := range []string{"mint", "configured", "surf", "prepared_fan_in", "fan_in", "prepared_raw", "raw_capacity", "capacity_recovery", "overload", "cleanup"} {
		phase, ok := report.Phase(name)
		if !ok || phase.Attempts == 0 || phase.Failures != 0 {
			t.Fatalf("phase %q = %+v, present=%t", name, phase, ok)
		}
	}
	if report.PhaseMust("configured").PreparedHits != 100 || report.PhaseMust("configured").P95MS < 0 {
		t.Fatalf("configured phase = %+v", report.PhaseMust("configured"))
	}
	if validator.calls < 5 {
		t.Fatalf("validator calls = %d, want raw capacity plus overload", validator.calls)
	}
	if fixture.HeldProgressAfterAdmission < 16 {
		t.Fatalf("held viewers made no meaningful post-admission progress: %d bytes", fixture.HeldProgressAfterAdmission)
	}
	heldContinuity := report.PhaseMust("overload").HeldContinuity
	if len(heldContinuity) != 4 {
		t.Fatalf("held continuity observations = %d, want 4", len(heldContinuity))
	}
	for _, held := range heldContinuity {
		if held.Outcome != "observed" || held.ObservationMS < 500 || held.AdvancingReads == 0 || held.BytesObserved == 0 || !held.DecodedFrame || held.Media.VideoStreams != 1 || held.Media.AudioStreams != 1 {
			t.Fatalf("held continuity evidence = %+v", held)
		}
	}
	var capacitySample *ResourceSample
	for index := range report.Resources {
		sample := &report.Resources[index]
		if sample.Point == "raw_capacity_in_phase" {
			capacitySample = sample
		}
	}
	if capacitySample == nil || capacitySample.TranscodeCost != capacitySample.Capacity || capacitySample.ViewerActive < capacitySample.Capacity {
		t.Fatalf("capacity was not observed while the full cohort was held: %+v", capacitySample)
	}
	for _, phase := range report.Phases {
		if phase.Resources.Samples == 0 || phase.Resources.SampleFailures != 0 {
			t.Fatalf("phase %q has no usable sampled resource evidence: %+v", phase.Name, phase.Resources)
		}
	}
	capacity := report.PhaseMust("raw_capacity").Resources.Maximum
	if capacity.TranscodeCost < report.Target.Capacity || capacity.ViewerActive < report.Target.Capacity {
		t.Fatalf("raw capacity sampled maximum = %+v", capacity)
	}
	blob, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var unfinalized minimalReport
	if err := json.Unmarshal(blob, &unfinalized); err != nil {
		t.Fatalf("unfinalized report JSON = %s: %v", blob, err)
	}
	if unfinalized.SchemaVersion != SchemaVersion || unfinalized.Certified || unfinalized.AuditStatus != AuditMissing || unfinalized.AuditReason != AuditReasonNotFinalized {
		t.Fatalf("unfinalized report boundary = %+v, want schema=%d uncertified/missing/not-finalized", unfinalized, SchemaVersion)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(blob, &fields); err != nil {
		t.Fatalf("unfinalized report fields = %s: %v", blob, err)
	}
	for field := range fields {
		if field != "schemaVersion" && field != "certified" && field != "auditStatus" && field != "auditReason" {
			t.Fatalf("unfinalized report exposed diagnostic field %q: %s", field, blob)
		}
	}
	unsafe := []string{fixture.Admin, fixture.Device, "signed-secret", "channel-private-", "Operator Library Title", fixture.Server.URL}
	for _, value := range unsafe {
		if strings.Contains(string(blob), value) {
			t.Fatalf("report leaked %q: %s", value, blob)
		}
	}
	if fixture.MaxConcurrentRaw < 4 {
		t.Fatalf("raw requests were not barrier-concurrent: peak=%d", fixture.MaxConcurrentRaw)
	}
	requireUncertifiedPublication(t, report)
}

func TestRunDoesNotCertifyAnInsufficientTranscodeCohort(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	channels := fixtureChannels(100)
	for i := 1; i < len(channels); i++ {
		channels[i].Roles = []string{"prepared"}
	}
	report, err := Run(context.Background(), fixtureConfig(fixture, channels))
	if err != nil {
		t.Fatal(err)
	}
	if report.PhaseMust("raw_capacity").HTTPClasses["transcode_cohort_insufficient"] == 0 {
		t.Fatalf("insufficient cohort omitted rejection: %+v", report)
	}
	requireUncertifiedPublication(t, report)
}

func TestRunDoesNotCertifyWhenOverloadIsAdmittedOrHeldViewerDrops(t *testing.T) {
	for _, tc := range []struct {
		name      string
		set       func(*playoutcertfixture.Fixture)
		wantClass string
	}{
		{name: "admitted", set: func(f *playoutcertfixture.Fixture) { f.AllowOverload = true }, wantClass: "admission_outcome_missing"},
		{name: "held viewer interrupted", set: func(f *playoutcertfixture.Fixture) { f.InterruptHeld = true }, wantClass: "held_viewer_interrupted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := playoutcertfixture.New(t, 100)
			tc.set(fixture)
			report, err := Run(context.Background(), fixtureConfig(fixture, fixtureChannels(100)))
			if err != nil {
				t.Fatal(err)
			}
			if report.PhaseMust("overload").HTTPClasses[tc.wantClass] == 0 {
				t.Fatalf("unsafe overload omitted %q: %+v", tc.wantClass, report.PhaseMust("overload"))
			}
			requireUncertifiedPublication(t, report)
		})
	}
}

func TestRunDoesNotTreatBufferedPreOverloadMediaAsHeldProgress(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	fixture.HeldBufferedBytes = 64 << 10
	fixture.StallHeldAfterOverload = true
	report, err := Run(context.Background(), fixtureConfig(fixture, fixtureChannels(100)))
	if err != nil {
		t.Fatal(err)
	}
	phase := report.PhaseMust("overload")
	if phase.HTTPClasses["held_viewer_interrupted"] == 0 {
		t.Fatalf("buffered pre-overload data omitted continuity failure: %+v", phase)
	}
	requireUncertifiedPublication(t, report)
}

func TestRunRecordsHeldDecodeFailure(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	config := fixtureConfig(fixture, fixtureChannels(100))
	config.Decoder = &playoutcertfixture.Decoder{Fail: true}
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	phase := report.PhaseMust("overload")
	if phase.HTTPClasses["held_viewer_interrupted"] == 0 {
		t.Fatalf("held decode failure omitted interruption: %+v", phase)
	}
	if len(phase.HeldContinuity) != 4 {
		t.Fatalf("held continuity observations = %d, want 4", len(phase.HeldContinuity))
	}
	for _, held := range phase.HeldContinuity {
		if held.Outcome != "decode_failed" || held.DecodedFrame {
			t.Fatalf("held decode evidence = %+v", held)
		}
	}
	requireUncertifiedPublication(t, report)
}

func TestRunRejectsValidInitialMediaFollowedByUndecodableHeldMedia(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	decoder := &playoutcertfixture.Decoder{FailAfter: fixture.CapacityRejected()}
	config := fixtureConfig(fixture, fixtureChannels(100))
	config.Decoder = decoder
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	rawCapacity := report.PhaseMust("raw_capacity")
	if rawCapacity.Attempts != 4 || rawCapacity.Failures != 0 || len(rawCapacity.Media) != 4 {
		t.Fatalf("initial raw-capacity media was not validated: %+v", rawCapacity)
	}
	for _, media := range rawCapacity.Media {
		if media.VideoStreams != 1 || media.AudioStreams != 1 {
			t.Fatalf("initial raw-capacity media = %+v", media)
		}
	}
	phase := report.PhaseMust("overload")
	if phase.HTTPClasses["held_viewer_interrupted"] == 0 {
		t.Fatalf("undecodable post-overload media omitted interruption: %+v", phase)
	}
	if len(phase.HeldContinuity) != 4 {
		t.Fatalf("held continuity observations = %d, want 4", len(phase.HeldContinuity))
	}
	for _, held := range phase.HeldContinuity {
		if held.Outcome != "decode_failed" || held.DecodedFrame {
			t.Fatalf("post-overload decode evidence = %+v", held)
		}
	}
	active, started, stopped := decoder.Counts()
	if active != 0 || started == 0 || stopped != started {
		t.Fatalf("decoder lifecycle active=%d started=%d stopped=%d", active, started, stopped)
	}
	requireUncertifiedPublication(t, report)
}

func TestRunCancellationBeforeHeldVerificationJoinsAllDecoders(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	decoder := &playoutcertfixture.Decoder{}
	ctx, cancel := context.WithCancel(context.Background())
	config := fixtureConfig(fixture, fixtureChannels(100))
	config.Decoder = decoder
	config.Client = fixture.CancelOnRejectedOverloadClient(cancel)
	report, err := Run(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	active, started, stopped := decoder.Counts()
	if active != 0 || started == 0 || stopped != started {
		t.Fatalf("decoder lifecycle active=%d started=%d stopped=%d", active, started, stopped)
	}
	requireUncertifiedPublication(t, report)
}

func TestRunDoesNotCertifyWhenPhaseResourceSamplingFails(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	fixture.FailOneMetricsAfterStart = true
	report, err := Run(context.Background(), fixtureConfig(fixture, fixtureChannels(100)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(report.Failures, ","), "resource_sample_failed") {
		t.Fatalf("sample failure was discarded: %+v", report.Failures)
	}
	if report.Resources[0].Point != "baseline" || report.Resources[len(report.Resources)-1].Point != "converged" {
		t.Fatalf("sampling must fail within a phase, not at the baseline/final checkpoints: %+v", report.Resources)
	}
	requireUncertifiedPublication(t, report)
}

func TestRunMarksLateParentFaultReceiptUnavailable(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	channels := fixtureChannels(100)
	config := fixtureConfig(fixture, channels)
	config.FaultProfiles = []FaultProfile{FaultParentFailure}
	config.FaultController = fixtureParentFaultController{target: playoutcertfixture.ParentFaultTarget{Fixture: fixture, Peer: channels[1].ID, WaitForExpiry: true}}
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	row := parentFaultRow(t, report)
	if row.Status != "unavailable" || row.ReceiptOutcome != "fault_budget_expired" {
		t.Fatalf("late parent receipt qualified or lacked bounded evidence: %+v", row)
	}
}

func TestRunShutdownRetainsReceiptEvidenceWhenFinalSamplingFails(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	channels := fixtureChannels(100)
	config := fixtureConfig(fixture, channels)
	config.FaultProfiles = []FaultProfile{FaultShutdown}
	config.FaultController = fixtureShutdownFaultController{target: shutdownTarget(fixture, channels, playoutcertfixture.ShutdownTarget{SampleErr: errors.New("shutdown metrics unavailable")})}
	config.DisposableTarget = config.FaultController.Scope()
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	row := shutdownFaultRow(t, report)
	if row.Status != "unavailable" || row.ReceiptOutcome != "exited" || row.Baseline == nil || row.PhasePeak == nil || row.Final != nil || !slices.Contains(report.Failures, "final_resource_sample_failed") {
		t.Fatalf("shutdown sampling failure discarded receipt evidence: row=%+v failures=%v", row, report.Failures)
	}
	requireUncertifiedPublication(t, report)
}

func TestRunShutdownLateReceiptCannotQualify(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	channels := fixtureChannels(100)
	config := fixtureConfig(fixture, channels)
	config.RequestTimeout = 10 * time.Millisecond
	config.FaultProfiles = []FaultProfile{FaultShutdown}
	config.FaultController = fixtureShutdownFaultController{target: shutdownTarget(fixture, channels, playoutcertfixture.ShutdownTarget{WaitForExpiry: true})}
	config.DisposableTarget = config.FaultController.Scope()
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	row := shutdownFaultRow(t, report)
	if row.Status != "unavailable" || row.ReceiptOutcome != "fault_budget_expired" || row.Final != nil || report.PhaseMust("shutdown").Failures == 0 {
		t.Fatalf("late shutdown receipt qualified: row=%+v phase=%+v", row, report.PhaseMust("shutdown"))
	}
}

func TestRunShutdownWithinBudgetRetainsObservedReceiptEligibility(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	channels := fixtureChannels(100)
	config := fixtureConfig(fixture, channels)
	config.FaultProfiles = []FaultProfile{FaultShutdown}
	config.FaultController = fixtureShutdownFaultController{target: shutdownTarget(fixture, channels, playoutcertfixture.ShutdownTarget{})}
	config.DisposableTarget = config.FaultController.Scope()
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	row := shutdownFaultRow(t, report)
	if row.ReceiptOutcome != "exited" || row.SelectedContinuity != "interrupted" || row.Status != "unavailable" || row.Outcome != "run_failed" || row.Baseline == nil || row.PhasePeak == nil || row.Final == nil {
		t.Fatalf("within-budget shutdown evidence = %+v", row)
	}
}

func TestRunShutdownRejectsUnavailableFinalSamples(t *testing.T) {
	tests := []struct {
		name   string
		sample playoutcertfixture.StoppedResource
	}{
		{name: "empty nil-error sample"},
		{name: "point-only incomplete sample", sample: playoutcertfixture.StoppedResource{Point: "shutdown_final"}},
		{name: "mismatched measured capacity", sample: playoutcertfixture.StoppedResource{Point: "shutdown_final", Capacity: 3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := playoutcertfixture.New(t, 100)
			channels := fixtureChannels(100)
			config := fixtureConfig(fixture, channels)
			config.CleanupTimeout = 10 * time.Millisecond
			config.FaultProfiles = []FaultProfile{FaultShutdown}
			config.FaultController = fixtureShutdownFaultController{target: shutdownTarget(fixture, channels, playoutcertfixture.ShutdownTarget{Sample: &tc.sample})}
			config.DisposableTarget = config.FaultController.Scope()
			report, err := Run(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			row := shutdownFaultRow(t, report)
			if row.Status != "unavailable" || row.ReceiptOutcome != "exited" || !slices.Contains(report.Failures, "final_resource_sample_failed") || report.PhaseMust("shutdown").Failures == 0 {
				t.Fatalf("unavailable final sample qualified: row=%+v failures=%v phase=%+v", row, report.Failures, report.PhaseMust("shutdown"))
			}
		})
	}
}

func TestRunShutdownInvalidSampleRetainsPriorMeasuredResidual(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	channels := fixtureChannels(100)
	config := fixtureConfig(fixture, channels)
	config.CleanupTimeout = 10 * time.Millisecond
	config.FaultProfiles = []FaultProfile{FaultShutdown}
	index := 0
	config.FaultController = fixtureShutdownFaultController{target: shutdownTarget(fixture, channels, playoutcertfixture.ShutdownTarget{
		Samples: []playoutcertfixture.StoppedResource{
			{Point: "shutdown_final", Capacity: 4, SessionsActive: 1},
			{},
		},
		SampleIndex: &index,
	})}
	config.DisposableTarget = config.FaultController.Scope()
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(report.Failures, "final_resource_sample_failed") || len(report.Resources) == 0 {
		t.Fatalf("missing failed residual evidence: failures=%v resources=%+v", report.Failures, report.Resources)
	}
	final := report.Resources[len(report.Resources)-1]
	if final.Point != "shutdown_final" || final.Capacity != 4 || final.SessionsActive != 1 {
		t.Fatalf("invalid sample replaced measured residual: %+v", final)
	}
}

func TestRunParentFaultCleanupResidualRetainsFinalEvidenceButDisqualifiesOutcome(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	channels := fixtureChannels(100)
	config := fixtureConfig(fixture, channels)
	config.FaultProfiles = []FaultProfile{FaultParentFailure}
	config.FaultController = fixtureParentFaultController{target: playoutcertfixture.ParentFaultTarget{Fixture: fixture, Peer: channels[1].ID, RetainAfterRecovery: true}}
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	row := parentFaultRow(t, report)
	if drill := report.PhaseMust("parent_failure"); drill.Failures != 0 || drill.Successes != 1 {
		t.Fatalf("parent drill did not succeed before cleanup invalidation: %+v", drill)
	}
	if row.Baseline == nil || row.Final == nil || row.Final.Capacity == 0 || row.Final.SessionsActive <= row.Baseline.SessionsActive || row.Status != "unavailable" || row.Outcome != "cleanup_failed" || !slices.Contains(report.Failures, "cleanup_residual") {
		t.Fatalf("Run cleanup residual fault evidence = %+v failures=%v", row, report.Failures)
	}
}

func TestRunChildFaultCleanupResidualRetainsFinalEvidenceButDisqualifiesOutcome(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	channels := fixtureChannels(100)
	config := fixtureConfig(fixture, channels)
	config.FaultProfiles = []FaultProfile{FaultChildFailure}
	config.FaultController = fixtureChildFaultController{target: playoutcertfixture.ParentFaultTarget{Fixture: fixture, Peer: channels[1].ID, RetainAfterRecovery: true}}
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	row := childFaultRow(t, report)
	if drill := report.PhaseMust("child_failure"); drill.Failures != 0 || drill.Successes != 1 {
		t.Fatalf("child drill did not succeed before cleanup invalidation: %+v", drill)
	}
	if row.Baseline == nil || row.Final == nil || row.Final.Capacity == 0 || row.Final.SessionsActive <= row.Baseline.SessionsActive || row.Status != "unavailable" || row.Outcome != "cleanup_failed" || !slices.Contains(report.Failures, "cleanup_residual") {
		t.Fatalf("Run child cleanup residual fault evidence = %+v failures=%v", row, report.Failures)
	}
}

func TestWaitCurrentChildRequiresLiveGenerationWithinDeadline(t *testing.T) {
	for _, tc := range []struct {
		name        string
		unavailable int32
		wantExpiry  bool
		poll        time.Duration
	}{
		{name: "between finite children", unavailable: 2},
		{name: "no later child", unavailable: -1, wantExpiry: true},
		{name: "coarse cleanup interval", unavailable: 2, poll: 250 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := &atomic.Int32{}
			controller := fixtureChildFaultController{target: playoutcertfixture.ParentFaultTarget{CurrentCalls: calls, UnavailableCalls: tc.unavailable}}
			timeout := 100 * time.Millisecond
			if tc.wantExpiry {
				timeout = 30 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(t.Context(), timeout)
			defer cancel()
			current, err := waitCurrentChild(ctx, controller, ChildFaultRequest{}, max(tc.poll, time.Millisecond))
			if tc.wantExpiry {
				if !errors.Is(err, context.DeadlineExceeded) || current != (ChildFaultTarget{}) {
					t.Fatalf("missing child qualified: %+v %v", current, err)
				}
			} else if err != nil || current.ParentGeneration != 1 || current.ChildGeneration != 1 || calls.Load() != 3 {
				t.Fatalf("later live child not selected: %+v %v calls=%d", current, err, calls.Load())
			}
		})
	}
}

func TestChildFailureDrillRefusesUnsupportedController(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	config := fixtureConfig(fixture, fixtureChannels(100))
	config.FaultController = playoutcertfixture.ScopedFaultController{ScopeName: "fixture-without-child-authority"}
	config = config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	drill := childFailureDrill(context.Background(), endpoint, config, []int{0, 1}, 2, nil)
	if drill.phase.Failures != 1 || drill.receipt != "not_observed" || drill.selected != "not_observed" || drill.peer != "not_observed" || drill.recovery != "not_observed" {
		t.Fatalf("unsupported controller qualified a child drill: %+v", drill)
	}
}

func TestParentFailureDrillUsesOnlySelectedAndRequiredPeer(t *testing.T) {
	for _, tc := range []struct {
		name         string
		capacity     int
		indexes      []int
		wantFailures int
		wantPeer     string
		wantSelected string
		wantReceipt  string
	}{
		{name: "capacity above two uses selected and peer", capacity: 4, indexes: []int{0, 1}, wantPeer: "continued", wantSelected: "interrupted", wantReceipt: "exited"},
		{name: "missing required peer fails closed", capacity: 2, indexes: []int{0}, wantFailures: 1, wantPeer: "not_observed", wantSelected: "not_observed", wantReceipt: "not_observed"},
		{name: "capacity one observes selected only", capacity: 1, indexes: []int{0}, wantPeer: "not_applicable", wantSelected: "interrupted", wantReceipt: "exited"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := playoutcertfixture.New(t, 100)
			channels := fixtureChannels(100)
			config := fixtureConfig(fixture, channels)
			config.FaultController = fixtureParentFaultController{target: playoutcertfixture.ParentFaultTarget{Fixture: fixture, Peer: channels[1].ID}}
			config = config.normalized()
			endpoint, err := newEndpoint(config)
			if err != nil {
				t.Fatal(err)
			}
			drill := parentFailureDrill(context.Background(), endpoint, config, tc.indexes, tc.capacity, nil)
			if drill.phase.Failures != tc.wantFailures || drill.peer != tc.wantPeer || drill.selected != tc.wantSelected || drill.receipt != tc.wantReceipt {
				t.Fatalf("parent drill = %+v", drill)
			}
		})
	}
}

type fixtureParentFaultController struct {
	target playoutcertfixture.ParentFaultTarget
}

type fixtureShutdownFaultController struct {
	target playoutcertfixture.ShutdownTarget
}

type fixtureChildFaultController struct {
	target playoutcertfixture.ParentFaultTarget
}

func (c fixtureChildFaultController) Scope() string { return "fixture-child-fault" }

func (c fixtureChildFaultController) CurrentChild(ctx context.Context, _ ChildFaultRequest) (ChildFaultTarget, error) {
	generation, err := c.target.Current(ctx)
	return ChildFaultTarget{ParentGeneration: generation, ChildGeneration: generation}, err
}

func (c fixtureChildFaultController) FailChild(ctx context.Context, request ChildFaultRequest) (ChildFaultReceipt, error) {
	if err := c.target.Fail(ctx, request.ChannelID); err != nil {
		return ChildFaultReceipt{}, err
	}
	return ChildFaultReceipt{ChannelID: request.ChannelID, ParentGeneration: request.ParentGeneration, ChildGeneration: request.ChildGeneration, Exited: true}, nil
}

func (c fixtureParentFaultController) Scope() string { return "fixture-parent-fault" }

func (c fixtureShutdownFaultController) Scope() string { return "fixture-shutdown-fault" }

func (c fixtureShutdownFaultController) Shutdown(ctx context.Context, _ ShutdownRequest) (ShutdownReceipt, error) {
	err := c.target.Stop(ctx)
	return ShutdownReceipt{Scope: c.Scope(), ServingStopped: err == nil, ProcessesExited: err == nil}, err
}

func (c fixtureShutdownFaultController) SampleStopped(ctx context.Context, point string) (ResourceSample, error) {
	sample, err := c.target.SampleStopped(ctx, point)
	return ResourceSample{Point: sample.Point, RSSBytes: sample.RSSBytes, CPUSeconds: sample.CPUSeconds, OpenFDs: sample.OpenFDs, Goroutines: sample.Goroutines, HTTPInFlight: sample.HTTPInFlight, SessionsActive: sample.SessionsActive, ViewerActive: sample.ViewerActive, GraceIdle: sample.GraceIdle, TranscodeCost: sample.TranscodeCost, Capacity: sample.Capacity, FFmpegRunning: sample.FFmpegRunning, PreparedChannels: sample.PreparedChannels, ReadyChannels: sample.ReadyChannels, ChannelHealth: sample.ChannelHealth, StalledChannels: sample.StalledChannels}, err
}

func shutdownTarget(fixture *playoutcertfixture.Fixture, channels []Channel, target playoutcertfixture.ShutdownTarget) playoutcertfixture.ShutdownTarget {
	target.Fixture = fixture
	target.Channels = make([]string, len(channels))
	for index := range channels {
		target.Channels[index] = channels[index].ID
	}
	if len(target.Samples) > 0 && target.SampleIndex == nil {
		target.SampleIndex = new(int)
	}
	return target
}

func (c fixtureParentFaultController) CurrentParent(ctx context.Context, _ ParentFaultRequest) (uint64, error) {
	return c.target.Current(ctx)
}

func (c fixtureParentFaultController) FailParent(ctx context.Context, request ParentFaultRequest) (ParentFaultReceipt, error) {
	if err := c.target.Fail(ctx, request.ChannelID); err != nil {
		return ParentFaultReceipt{}, err
	}
	return ParentFaultReceipt{ChannelID: request.ChannelID, Generation: request.Generation, Exited: true}, nil
}

func parentFaultRow(t testing.TB, report Report) FaultQualification {
	t.Helper()
	for _, row := range report.FaultProfiles {
		if row.Profile == FaultParentFailure {
			return row
		}
	}
	t.Fatal("parent fault qualification missing")
	return FaultQualification{}
}

func shutdownFaultRow(t testing.TB, report Report) FaultQualification {
	t.Helper()
	for _, row := range report.FaultProfiles {
		if row.Profile == FaultShutdown {
			return row
		}
	}
	t.Fatal("shutdown fault qualification missing")
	return FaultQualification{}
}

func childFaultRow(t testing.TB, report Report) FaultQualification {
	t.Helper()
	for _, row := range report.FaultProfiles {
		if row.Profile == FaultChildFailure {
			return row
		}
	}
	t.Fatal("child fault qualification missing")
	return FaultQualification{}
}

func fixtureConfig(fixture *playoutcertfixture.Fixture, channels []Channel) Config {
	validator := &recordingValidator{}
	decoder := &playoutcertfixture.Decoder{}
	return Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, DeviceToken: fixture.Device,
		Channels: channels, Certify: true, RemoteAcknowledged: true, Concurrency: 12, SurfRounds: 1,
		FanInViewers: 4, RequestTimeout: time.Second, CleanupTimeout: time.Second, CleanupPoll: time.Millisecond,
		RawCaptureBytes: 188, Validator: validator, Decoder: decoder}
}

func TestRawBurstValidatesDecodedCaptureWithoutMinimumByteCount(t *testing.T) {
	for _, audioStreams := range []int{1, 0} {
		t.Run(fmt.Sprintf("audio-streams-%d", audioStreams), func(t *testing.T) {
			fixture := playoutcertfixture.New(t, 1)
			channels := fixtureChannels(1)
			fixture.ContinuousRawChannels = map[string]bool{channels[0].ID: true}
			config := fixtureConfig(fixture, channels)
			config.RawCaptureBytes = 2 << 20
			config.RequestTimeout = 500 * time.Millisecond
			config.Validator = &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: audioStreams, VideoCodec: "h264", AudioCodec: "aac"}}}
			config = config.normalized()
			endpoint, err := newEndpoint(config)
			if err != nil {
				t.Fatal(err)
			}
			results, _ := rawBurst(t.Context(), endpoint, config, []int{0})
			want := "ok"
			if audioStreams == 0 {
				want = "invalid_media"
			}
			if len(results) != 1 || results[0].class != want {
				t.Fatalf("decoded low-rate capture = %+v, want %s", results, want)
			}
		})
	}
}

func TestNearestRankPercentilesKeepFailuresSeparate(t *testing.T) {
	got := summarize([]time.Duration{10 * time.Millisecond, 40 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}, 2)
	if got.Attempts != 6 || got.Successes != 4 || got.Failures != 2 || got.P50MS != 20 || got.P95MS != 40 || got.P99MS != 40 {
		t.Fatalf("summary = %+v", got)
	}
}

type recordingValidator struct {
	mu    sync.Mutex
	calls int
}

func (v *recordingValidator) Validate(_ context.Context, body []byte) (MediaShape, error) {
	v.mu.Lock()
	v.calls++
	v.mu.Unlock()
	if len(body) == 0 {
		return MediaShape{}, fmt.Errorf("empty body")
	}
	return MediaShape{VideoStreams: 1, AudioStreams: 1, VideoCodec: "h264", AudioCodec: "aac"}, nil
}

func fixtureChannels(n int) []Channel {
	out := make([]Channel, n)
	for i := range out {
		out[i] = Channel{ID: fmt.Sprintf("channel-private-%03d", i+1), Roles: []string{"prepared"}}
	}
	for i := range min(n, 5) {
		out[i].Roles = append(out[i].Roles, "transcode_h264", "audio_aac")
	}
	return out
}

func TestRawBurstDefersMetadataUntilEveryViewerFinishesStartup(t *testing.T) {
	fixture := playoutcertfixture.New(t, 2)
	channels := fixtureChannels(2)
	fixture.ContinuousRawChannels = map[string]bool{channels[0].ID: true, channels[1].ID: true}
	gate := make(chan struct{})
	waiting := make(chan int, 1)
	validation := make(chan int, 2)
	decoder := &playoutcertfixture.Decoder{FirstFrameGate: gate, FirstFrameGateCall: 2, FirstFrameWaiting: waiting}
	validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}, CallStarted: validation}
	config := fixtureConfig(fixture, channels)
	config.Decoder = decoder
	config.Validator = validator
	config = config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan []observation, 1)
	go func() { results, _ := rawBurst(t.Context(), endpoint, config, []int{0, 1}); finished <- results }()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("second viewer did not reach frame gate")
	}
	early := false
	select {
	case <-validation:
		early = true
	case <-time.After(100 * time.Millisecond):
	}
	close(gate)
	results := <-finished
	if early {
		t.Error("metadata validator ran while another viewer was still awaiting its first frame")
	}
	if len(results) != 2 || results[0].class != "ok" || results[1].class != "ok" || validator.Calls() != 2 {
		t.Fatalf("results=%+v validations=%d", results, validator.Calls())
	}
	active, started, stopped := decoder.Counts()
	if active != 0 || started != 2 || stopped != 2 {
		t.Fatalf("decoder lifecycle=%d/%d/%d", active, started, stopped)
	}
}

func TestRawBurstStartupFailureReleasesPeers(t *testing.T) {
	for _, kind := range []string{"open", "decode"} {
		t.Run(kind, func(t *testing.T) {
			count := 2
			if kind == "open" {
				count = 5
			}
			fixture := playoutcertfixture.New(t, count)
			channels := fixtureChannels(count)
			fixture.ContinuousRawChannels = map[string]bool{}
			indexes := make([]int, count)
			for i, ch := range channels {
				fixture.ContinuousRawChannels[ch.ID] = true
				indexes[i] = i
			}
			decoder := &playoutcertfixture.Decoder{}
			wantClass := "http_503"
			wantDecoders := 4
			wantValidations := 4
			if kind == "decode" {
				decoder.FirstFrameFailCall = 2
				wantClass = "decode_failed"
				wantDecoders = 2
				wantValidations = 1
			}
			validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}}
			config := fixtureConfig(fixture, channels)
			config.Decoder = decoder
			config.Validator = validator
			config = config.normalized()
			endpoint, err := newEndpoint(config)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan []observation, 1)
			go func() { results, _ := rawBurst(t.Context(), endpoint, config, indexes); done <- results }()
			select {
			case results := <-done:
				classes := make([]string, len(results))
				for i, result := range results {
					classes[i] = result.class
				}
				slices.Sort(classes)
				want := []string{wantClass}
				for range wantValidations {
					want = append(want, "ok")
				}
				slices.Sort(want)
				if !slices.Equal(classes, want) || validator.Calls() != wantValidations {
					t.Fatalf("classes=%v validations=%d", classes, validator.Calls())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("startup failure did not release barrier")
			}
			active, started, stopped := decoder.Counts()
			if active != 0 || started != wantDecoders || stopped != wantDecoders {
				t.Fatalf("decoder lifecycle=%d/%d/%d", active, started, stopped)
			}
		})
	}
}

func TestRawBurstStartupBarrierPreservesCancellationAndOriginalDeadline(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel-%t", cancelEarly), func(t *testing.T) {
			fixture := playoutcertfixture.New(t, 2)
			channels := fixtureChannels(2)
			fixture.ContinuousRawChannels = map[string]bool{channels[0].ID: true, channels[1].ID: true}
			gate := make(chan struct{})
			waiting := make(chan int, 1)
			decoder := &playoutcertfixture.Decoder{FirstFrameGate: gate, FirstFrameGateCall: 2, FirstFrameWaiting: waiting}
			validator := &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1}}}
			config := fixtureConfig(fixture, channels)
			config.Decoder = decoder
			config.Validator = validator
			config.RequestTimeout = 150 * time.Millisecond
			config = config.normalized()
			endpoint, err := newEndpoint(config)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan []observation, 1)
			go func() { results, _ := rawBurst(ctx, endpoint, config, []int{0, 1}); done <- results }()
			select {
			case <-waiting:
			case <-time.After(time.Second):
				t.Fatal("viewer did not reach startup gate")
			}
			if cancelEarly {
				cancel()
			}
			select {
			case results := <-done:
				classes := []string{results[0].class, results[1].class}
				slices.Sort(classes)
				if !slices.Equal(classes, []string{"body_failed", "decode_failed"}) || validator.Calls() != 0 {
					t.Fatalf("classes=%v validations=%d", classes, validator.Calls())
				}
			case <-time.After(time.Second):
				t.Fatal("startup barrier ignored cancellation/deadline")
			}
			active, started, stopped := decoder.Counts()
			if active != 0 || started != 2 || stopped != 2 {
				t.Fatalf("decoder lifecycle=%d/%d/%d", active, started, stopped)
			}
		})
	}
}
