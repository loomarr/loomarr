package playoutcert

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

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
	if report.Certified || !strings.Contains(strings.Join(report.Failures, ","), "programme_boundary_failed") {
		t.Fatalf("ordinary fixture unexpectedly certified without causal boundary evidence: %+v", report.Failures)
	}
	boundary := report.PhaseMust("programme_boundary")
	// The ordinary fixture serves no decodable prepared-HLS epoch.  It still
	// cannot certify without causal boundary evidence, but the public attempt
	// now reaches the decoder rather than being classified as unavailable.
	if boundary.HTTPClasses["decode_failed"] != 1 || boundary.HTTPClasses["cohort_missing"] != 1 {
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
	unsafe := []string{fixture.Admin, fixture.Device, "signed-secret", "channel-private-", "Operator Library Title", fixture.Server.URL}
	for _, value := range unsafe {
		if strings.Contains(string(blob), value) {
			t.Fatalf("report leaked %q: %s", value, blob)
		}
	}
	if fixture.MaxConcurrentRaw < 4 {
		t.Fatalf("raw requests were not barrier-concurrent: peak=%d", fixture.MaxConcurrentRaw)
	}
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
	if report.Certified || report.PhaseMust("raw_capacity").HTTPClasses["transcode_cohort_insufficient"] == 0 {
		t.Fatalf("insufficient cohort certified: %+v", report)
	}
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
			if report.Certified || report.PhaseMust("overload").HTTPClasses[tc.wantClass] == 0 {
				t.Fatalf("unsafe overload certified: %+v", report.PhaseMust("overload"))
			}
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
	if report.Certified || phase.HTTPClasses["held_viewer_interrupted"] == 0 {
		t.Fatalf("buffered pre-overload data passed continuity: %+v", phase)
	}
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
	if report.Certified || phase.HTTPClasses["held_viewer_interrupted"] == 0 {
		t.Fatalf("held decode failure certified: %+v", phase)
	}
	if len(phase.HeldContinuity) != 4 {
		t.Fatalf("held continuity observations = %d, want 4", len(phase.HeldContinuity))
	}
	for _, held := range phase.HeldContinuity {
		if held.Outcome != "decode_failed" || held.DecodedFrame {
			t.Fatalf("held decode evidence = %+v", held)
		}
	}
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
	if report.Certified || phase.HTTPClasses["held_viewer_interrupted"] == 0 {
		t.Fatalf("undecodable post-overload media certified: %+v", phase)
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
	if report.Certified {
		t.Fatal("cancelled certification unexpectedly passed")
	}
	active, started, stopped := decoder.Counts()
	if active != 0 || started == 0 || stopped != started {
		t.Fatalf("decoder lifecycle active=%d started=%d stopped=%d", active, started, stopped)
	}
}

func TestRunDoesNotCertifyWhenPhaseResourceSamplingFails(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	fixture.FailOneMetricsAfterStart = true
	report, err := Run(context.Background(), fixtureConfig(fixture, fixtureChannels(100)))
	if err != nil {
		t.Fatal(err)
	}
	if report.Certified || !strings.Contains(strings.Join(report.Failures, ","), "resource_sample_failed") {
		t.Fatalf("sample failure was discarded: %+v", report.Failures)
	}
	if report.Resources[0].Point != "baseline" || report.Resources[len(report.Resources)-1].Point != "converged" {
		t.Fatalf("sampling must fail within a phase, not at the baseline/final checkpoints: %+v", report.Resources)
	}
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

func (c fixtureParentFaultController) Scope() string { return "fixture-parent-fault" }

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

func fixtureConfig(fixture *playoutcertfixture.Fixture, channels []Channel) Config {
	validator := &recordingValidator{}
	decoder := &playoutcertfixture.Decoder{}
	return Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, DeviceToken: fixture.Device,
		Channels: channels, Certify: true, RemoteAcknowledged: true, Concurrency: 12, SurfRounds: 1,
		FanInViewers: 4, RequestTimeout: time.Second, CleanupTimeout: time.Second, CleanupPoll: time.Millisecond,
		RawCaptureBytes: 188, Validator: validator, Decoder: decoder}
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
