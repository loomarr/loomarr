//go:build ffmpeg

package playoutcert

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestSyntheticTargetCertifiesHundredPreparedChannelsAndBoundedTranscodeBurst(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := make([]Channel, 0, 105)
	for index := range 100 {
		channels = append(channels, Channel{ID: fmt.Sprintf("prepared-%03d", index+1), Roles: []string{"prepared"}})
	}
	for index := range 5 {
		channels = append(channels, Channel{ID: fmt.Sprintf("transcode-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	target, err := NewSyntheticTarget(ctx, SyntheticConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 4, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if closeErr := target.Close(closeCtx); closeErr != nil {
			t.Errorf("close synthetic target: %v", closeErr)
		}
	}()
	config := Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Channels: channels, Certify: true, Concurrency: 12, SurfRounds: 1, FanInViewers: 4,
		RequestTimeout: 15 * time.Second, CleanupTimeout: 10 * time.Second, CleanupPoll: 25 * time.Millisecond,
		WarmGrace:       time.Second,
		RawCaptureBytes: 2 << 20, PreparedP95: 100 * time.Millisecond,
		ProgrammeBoundaryTimeout: 15 * time.Second, ProgrammeBoundaryLateObservation: time.Second,
		ProgrammeBoundaryWitness: target.ProgrammeBoundaryWitness(),
		FaultProfiles:            []FaultProfile{FaultParentFailure},
		FaultController:          target,
		Validator:                FFprobeValidator{}, Decoder: FFmpegDecoder{},
	}
	report, err := Run(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Certified {
		t.Fatalf("certification failed: %v\n%sresources=%+v", report.Failures, HumanSummary(report), report.Resources)
	}
	parentFault := report.PhaseMust("parent_failure")
	if parentFault.Failures != 0 || parentFault.Attempts != 1 || len(parentFault.Media) != 1 {
		t.Fatalf("parent-failure recovery evidence = %+v", parentFault)
	}
	for _, row := range report.FaultProfiles {
		if row.Profile == FaultParentFailure {
			if row.Status != "qualified" || row.Outcome != "complete" || row.Baseline == nil || row.PhasePeak == nil || row.Final == nil || row.ReceiptOutcome != "exited" || row.SelectedContinuity != "interrupted" || row.PeerContinuity != "continued" || row.Recovery != "recovered" {
				t.Fatalf("parent-failure qualification = %+v", row)
			}
		}
	}
	if report.PhaseMust("configured").PreparedHits != 100 {
		t.Fatalf("prepared hits = %d", report.PhaseMust("configured").PreparedHits)
	}
	if configured := report.PhaseMust("configured"); configured.Attempts != len(channels) || configured.HTTPClasses["prepared_miss"] != 5 {
		t.Fatalf("configured catalog/cold outcomes = %+v", configured)
	}
	boundary := report.PhaseMust("programme_boundary")
	if boundary.Attempts != 2 || boundary.Failures != 0 || len(boundary.ProgrammeBoundaries) != 2 {
		t.Fatalf("programme boundary phase = %+v", boundary)
	}
	for _, evidence := range boundary.ProgrammeBoundaries {
		if evidence.Outcome != "ok" || evidence.Transitions != 1 || evidence.DecodedFrameDelta <= 0 || evidence.ReadDelta <= 0 || evidence.BytesDelta <= 0 || evidence.Media.VideoStreams != 1 || evidence.Media.AudioStreams != 1 {
			t.Fatalf("programme boundary evidence = %+v", evidence)
		}
	}
	if preparedRaw := report.PhaseMust("prepared_raw"); preparedRaw.Attempts != 4 || preparedRaw.Failures != 0 || preparedRaw.P95MS > 500 {
		t.Fatalf("prepared raw phase = %+v", preparedRaw)
	}
	if fanIn := report.PhaseMust("fan_in"); fanIn.Attempts != 4 || fanIn.Failures != 0 {
		t.Fatalf("fan-in phase = %+v", fanIn)
	}
	for _, name := range []string{"cancellation", "warm_reuse", "grace_expiry"} {
		if phase := report.PhaseMust(name); phase.Attempts != 1 || phase.Failures != 0 {
			t.Fatalf("%s phase = %+v", name, phase)
		}
	}
	if report.PhaseMust("overload").HTTPClasses["http_503"] == 0 {
		t.Fatalf("overload was not bounded: %+v", report.PhaseMust("overload"))
	}
	held := report.PhaseMust("overload").HeldContinuity
	if len(held) != 4 {
		t.Fatalf("held continuity observations = %d, want 4", len(held))
	}
	for _, observation := range held {
		if observation.Outcome != "observed" || !observation.DecodedFrame || observation.Media.VideoStreams != 1 || observation.Media.AudioStreams != 1 {
			t.Fatalf("held continuity evidence = %+v", observation)
		}
	}
}

func TestSyntheticTargetRejectsPostOverloadCorruptHeldMedia(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := make([]Channel, 0, 105)
	for index := range 100 {
		channels = append(channels, Channel{ID: fmt.Sprintf("prepared-%03d", index+1), Roles: []string{"prepared"}})
	}
	for index := range 5 {
		channels = append(channels, Channel{ID: fmt.Sprintf("transcode-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	heldStreams := make([]string, 4)
	for index := range heldStreams {
		heldStreams[index] = fmt.Sprintf("transcode-%03d", index+1)
	}
	target, err := NewSyntheticTarget(ctx, SyntheticConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 4, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if closeErr := target.Close(closeCtx); closeErr != nil {
			t.Errorf("close synthetic target: %v", closeErr)
		}
	}()
	config := Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Channels: channels, Certify: true, Concurrency: 12, SurfRounds: 1, FanInViewers: 4,
		RequestTimeout: 15 * time.Second, CleanupTimeout: 10 * time.Second, CleanupPoll: 25 * time.Millisecond,
		WarmGrace:       time.Second,
		RawCaptureBytes: 2 << 20, PreparedP95: 100 * time.Millisecond,
		ProgrammeBoundaryTimeout: 15 * time.Second, ProgrammeBoundaryLateObservation: time.Second,
		ProgrammeBoundaryWitness: target.ProgrammeBoundaryWitness(),
		Validator:                FFprobeValidator{}, Decoder: FFmpegDecoder{},
		Client: playoutcertfixture.PostEventCorruptClient(http.DefaultTransport.(*http.Transport).Clone(), heldStreams),
	}
	report, err := Run(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	rawCapacity := report.PhaseMust("raw_capacity")
	if rawCapacity.Failures != 0 || len(rawCapacity.Media) != 4 {
		t.Fatalf("initial raw-capacity media was not valid: %+v", rawCapacity)
	}
	overload := report.PhaseMust("overload")
	if report.Certified || overload.HTTPClasses["http_503"] != 1 || len(overload.HeldContinuity) != 4 {
		t.Fatalf("corrupt post-event run did not fail closed: %+v", overload)
	}
	for _, held := range overload.HeldContinuity {
		if held.Outcome == "observed" || held.DecodedFrame || held.BytesObserved == 0 {
			t.Fatalf("corrupt post-event media passed: %+v", held)
		}
	}
}
