package playoutcert

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestRunCopyBatchesDoNotUseVideoCapacity(t *testing.T) {
	fixture := playoutcertfixture.New(t, 7)
	channels := []Channel{{ID: "prepared-private", Roles: []string{"prepared"}}}
	fixture.PreparedMissChannels = map[string]bool{}
	fixture.ZeroCostChannels = map[string]bool{}
	fixture.ContinuousRawChannels = map[string]bool{}
	for index := range 6 {
		id := fmt.Sprintf("ordinary-private-%d", index)
		channels = append(channels, Channel{ID: id, Roles: []string{"copy"}})
		fixture.PreparedMissChannels[id] = true
		fixture.ZeroCostChannels[id] = true
		fixture.ContinuousRawChannels[id] = true
	}
	config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, DeviceToken: fixture.Device, Channels: channels,
		Concurrency: 3, FanInViewers: 1, RequestTimeout: time.Second, CleanupTimeout: time.Second,
		CleanupPoll: time.Millisecond, WarmGrace: time.Millisecond, RawCaptureBytes: 188,
		Validator: &recordingValidator{}, Decoder: &playoutcertfixture.Decoder{},
	}
	report, err := Run(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	phase := report.PhaseMust("copy_raw")
	if phase.Attempts != 6 || phase.Successes != 6 || phase.Failures != 0 || len(phase.HeldContinuity) != 6 || phase.Resources.Maximum.TranscodeCost != 0 || phase.Resources.Maximum.SessionsActive != 3 {
		t.Fatalf("bounded copy cohort: %+v", phase)
	}
	for _, held := range phase.HeldContinuity {
		if held.Outcome != "observed" || !held.DecodedFrame || held.AdvancingReads == 0 {
			t.Fatalf("copy viewer did not advance: %+v", held)
		}
	}
	if report.PhaseMust("cleanup").Failures != 0 {
		t.Fatal("copy batch resources did not converge")
	}
}

func TestCopyWorkloadRejectsCostAfterInitialMedia(t *testing.T) {
	fixture := playoutcertfixture.New(t, 1)
	const id = "ordinary-private"
	fixture.ZeroCostChannels = map[string]bool{id: true}
	fixture.ContinuousRawChannels = map[string]bool{id: true}
	fixture.RawOpened = make(chan struct{}, 1)
	config := (Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, DeviceToken: fixture.Device,
		Channels: []Channel{{ID: id, Roles: []string{"copy"}}}, Concurrency: 1, RequestTimeout: 2 * time.Second,
		CleanupTimeout: time.Second, CleanupPoll: 5 * time.Millisecond, RawCaptureBytes: 188,
		Validator: &recordingValidator{}, Decoder: &playoutcertfixture.Decoder{},
	}).normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := endpoint.sample(t.Context(), "baseline")
	if err != nil {
		t.Fatal(err)
	}
	changed := make(chan struct{})
	go func() {
		defer close(changed)
		select {
		case <-fixture.RawOpened:
		case <-t.Context().Done():
			return
		}
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			fixture.SetZeroCost(id, false)
		case <-t.Context().Done():
		}
	}()
	phase, samples := copyWorkload(t.Context(), endpoint, config, []int{0}, []observation{{class: "prepared_miss"}}, baseline)
	<-changed
	initialZero, heldCost := false, false
	for _, sample := range samples {
		initialZero = initialZero || (sample.Point == "raw_capacity" && sample.TranscodeCost == 0 && sample.ViewerActive == 1)
		heldCost = heldCost || (sample.Point == "copy_raw_held" && sample.TranscodeCost == 1)
	}
	if !initialZero || !heldCost || phase.HTTPClasses["copy_cost_mismatch"] == 0 || phase.Failures == 0 {
		t.Fatalf("later cost escaped initial-only accounting: %+v / %+v", phase, samples)
	}
}

func TestRunMeasuresCopyLaneAndPreservesPreparedRequirements(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		roles                  []string
		preparedMiss, zeroCost bool
		wantCopyClass          string
		wantCatalogFailure     bool
	}{
		{name: "ordinary copy", roles: []string{"copy", "audio_aac"}, preparedMiss: true, zeroCost: true},
		{name: "copy requiring encoding", roles: []string{"copy"}, preparedMiss: true, wantCopyClass: "copy_cost_mismatch"},
		{name: "copy backed by prepared publication", roles: []string{"copy"}, zeroCost: true, wantCopyClass: "copy_source_prepared"},
		{name: "prepared copy must hit", roles: []string{"prepared", "copy"}, preparedMiss: true, zeroCost: true, wantCatalogFailure: true},
		{name: "prepared transcode must hit", roles: []string{"prepared", "transcode_h264"}, preparedMiss: true, wantCopyClass: "cohort_missing", wantCatalogFailure: true},
		{name: "audio label cannot excuse missing prepared media", roles: []string{"audio_eac3"}, preparedMiss: true, wantCopyClass: "cohort_missing", wantCatalogFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := playoutcertfixture.New(t, 2)
			fixture.PreparedMissChannels = map[string]bool{"ordinary-private": tc.preparedMiss}
			fixture.ZeroCostChannels = map[string]bool{"ordinary-private": tc.zeroCost}
			fixture.ContinuousRawChannels = map[string]bool{"ordinary-private": true}
			config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, DeviceToken: fixture.Device,
				Channels:    []Channel{{ID: "prepared-private", Roles: []string{"prepared"}}, {ID: "ordinary-private", Roles: tc.roles}},
				Concurrency: 1, SurfRounds: 2, FanInViewers: 1, RequestTimeout: time.Second,
				CleanupTimeout: time.Second, CleanupPoll: time.Millisecond, WarmGrace: time.Millisecond,
				RawCaptureBytes: 188, Validator: &recordingValidator{}, Decoder: &playoutcertfixture.Decoder{},
			}
			report, err := Run(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"configured", "surf"} {
				phase := report.PhaseMust(name)
				if (phase.Failures > 0) != tc.wantCatalogFailure {
					t.Fatalf("%s = %+v", name, phase)
				}
				wantAttempts := 2
				if name == "surf" {
					wantAttempts = 4
				}
				if phase.Attempts != wantAttempts {
					t.Fatalf("%s omitted catalog entries: %+v", name, phase)
				}
				if tc.preparedMiss && phase.HTTPClasses["prepared_miss"] != wantAttempts/2 {
					t.Fatalf("%s lost miss evidence: %+v", name, phase)
				}
			}
			phase := report.PhaseMust("copy_raw")
			if tc.wantCopyClass != "" {
				if phase.HTTPClasses[tc.wantCopyClass] == 0 || phase.Failures == 0 || !slices.Contains(report.Failures, "copy_raw_failed") {
					t.Fatalf("copy assertion missing: %+v failures=%v", phase, report.Failures)
				}
			} else if phase.Failures != 0 || phase.Attempts != 1 || len(phase.HeldContinuity) != 1 || !phase.HeldContinuity[0].DecodedFrame || phase.HeldContinuity[0].AdvancingReads == 0 || phase.Resources.Maximum.TranscodeCost != 0 {
				t.Fatalf("copy not measured: %+v", phase)
			}
			// This generic target deliberately signs with exp=1; the private
			// value collides with numeric report fields and cannot publish detail.
			requireUncertifiedPublication(t, report)
		})
	}
}

func TestCopyReportVocabularyDoesNotExcuseDynamicCollisions(t *testing.T) {
	config := Config{BaseURL: "http://fixture.invalid", AdminBearer: "fixture-admin-secret", DeviceToken: "fixture-device-secret"}
	capsule := newAuditCapsule(config)
	classes := []string{"copy_cost_mismatch", "copy_session_missing", "copy_source_prepared", "cohort_missing"}
	phase := phaseFrom("copy_raw", nil)
	capsule.registerSource("copy_raw", probeCollision)
	for _, class := range classes {
		phase.HTTPClasses[class] = 1
		capsule.registerSource(class, probeCollision)
	}
	report := Report{SchemaVersion: SchemaVersion, Phases: []Phase{phase}, Failures: []string{"copy_raw_failed"}}
	for _, collision := range []bool{false, true} {
		if collision {
			report.Target.Version = "copy_raw"
		}
		data, err := marshalDiagnosticReport(report)
		if err != nil {
			t.Fatal(err)
		}
		document, reason, ok := parseJSONProvenance(data, false, newAuditWork(time.Now().Add(time.Second)))
		if !ok {
			t.Fatal(reason)
		}
		status, reason := auditDocuments(capsule, time.Now().Add(time.Second), document, renderReportSummary(report))
		if (!collision && status != AuditPassed) || (collision && (status != AuditUnavailable || reason != AuditReasonDynamicCollision)) {
			t.Fatalf("collision=%t: %s / %s", collision, status, reason)
		}
	}
}
