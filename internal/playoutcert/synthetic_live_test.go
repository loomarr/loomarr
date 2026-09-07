//go:build ffmpeg

package playoutcert

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
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

// This is deliberately independent of SyntheticTarget's raw BlockSource
// witness.  It proves the ordinary public prepared route can itself carry a
// decoder across the renderer's discontinuity/map/PDT programme boundary.
func TestSyntheticPreparedPublicHLSWitnessesProgrammeBoundaryWithoutRawWitness(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := []Channel{{ID: "prepared-boundary", Roles: []string{"prepared"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	target, err := NewSyntheticTarget(ctx, SyntheticConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 4 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if err := target.Close(closeCtx); err != nil {
			t.Error(err)
		}
	}()
	config := Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels, RequestTimeout: 10 * time.Second, RawCaptureBytes: 256 << 10, ProgrammeBoundaryTimeout: 20 * time.Second, ProgrammeBoundaryLateObservation: 3 * time.Second, Validator: FFprobeValidator{}, Decoder: FFmpegDecoder{}}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	result := observeProgrammeBoundary(ctx, endpoint, config.normalized(), boundaryLane{name: "prepared", channelIndex: 0})
	if result.observation.class != "ok" || result.evidence.Transitions != 1 || result.evidence.DecodedFrameDelta <= 0 || result.evidence.ReadDelta <= 0 {
		t.Fatalf("prepared public boundary=%+v", result)
	}
}

func TestSyntheticPreparedPublicHLSContinuesAcrossFurtherProgrammeEpochs(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := []Channel{{ID: "prepared-continuous-epochs", Roles: []string{"prepared"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	target, err := NewSyntheticTarget(ctx, SyntheticConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 4 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if err := target.Close(closeCtx); err != nil {
			t.Error(err)
		}
	}()
	config := Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels, RequestTimeout: 10 * time.Second, RawCaptureBytes: 256 << 10, ProgrammeBoundaryTimeout: 20 * time.Second, ProgrammeBoundaryLateObservation: 6 * time.Second, Validator: FFprobeValidator{}, Decoder: FFmpegDecoder{}}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	result := observeProgrammeBoundary(ctx, endpoint, config.normalized(), boundaryLane{name: "prepared", channelIndex: 0})
	if result.observation.class != "ok" || result.evidence.Transitions < 2 || result.evidence.DecodedFrameDelta <= 0 || result.evidence.ReadDelta <= 0 || result.evidence.BytesDelta <= 0 {
		t.Fatalf("prepared public continuous epochs=%+v", result)
	}
}

func TestSyntheticTargetParentFaultCertifiesAtOneMeasuredSlot(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := make([]Channel, 0, 101)
	for index := range 99 {
		channels = append(channels, Channel{ID: fmt.Sprintf("prepared-%03d", index+1), Roles: []string{"prepared"}})
	}
	for index := range 2 {
		channels = append(channels, Channel{ID: fmt.Sprintf("transcode-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	target, err := NewSyntheticTarget(ctx, SyntheticConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
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
	report, err := Run(ctx, Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Channels: channels, Certify: true, Concurrency: 12, SurfRounds: 1, FanInViewers: 1,
		RequestTimeout: 15 * time.Second, CleanupTimeout: 10 * time.Second, CleanupPoll: 25 * time.Millisecond,
		WarmGrace: time.Second, RawCaptureBytes: 2 << 20, PreparedP95: 100 * time.Millisecond,
		ProgrammeBoundaryTimeout: 15 * time.Second, ProgrammeBoundaryLateObservation: time.Second,
		ProgrammeBoundaryWitness: target.ProgrammeBoundaryWitness(), FaultProfiles: []FaultProfile{FaultParentFailure}, FaultController: target,
		Validator: FFprobeValidator{}, Decoder: FFmpegDecoder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Certified || strings.Contains(strings.Join(report.Failures, ","), "capacity_oversubscribed") {
		t.Fatalf("one-slot certification = failures %v resources=%+v", report.Failures, report.Resources)
	}
	for _, row := range report.FaultProfiles {
		if row.Profile == FaultParentFailure && (row.Status != "qualified" || row.PeerContinuity != "not_applicable" || row.SelectedContinuity != "interrupted" || row.Recovery != "recovered") {
			t.Fatalf("one-slot parent evidence = %+v", row)
		}
	}
}

func TestSyntheticTargetRejectsStaleParentGenerationAfterReplacement(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	channel := Channel{ID: "transcode", Roles: []string{"transcode_h264", "audio_aac"}}
	target, err := NewSyntheticTarget(ctx, SyntheticConfig{Channels: []Channel{channel}, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second})
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
	config := Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: []Channel{channel}, RequestTimeout: 15 * time.Second, RawCaptureBytes: 2 << 20, Validator: FFprobeValidator{}, Decoder: FFmpegDecoder{}}
	config = config.normalized()
	endpoint, err := newEndpoint(config)
	if err != nil {
		t.Fatal(err)
	}
	first, sample := rawBurst(ctx, endpoint, config, []int{0})
	if len(first) != 1 || first[0].class != "ok" || sample.Capacity == 0 {
		t.Fatalf("initial public media = observations %+v sample=%+v", first, sample)
	}
	request := ParentFaultRequest{BaseURL: target.BaseURL, ChannelID: channel.ID}
	generation, err := target.CurrentParent(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	request.Generation = generation
	if receipt, err := target.FailParent(ctx, request); err != nil || !receipt.Exited {
		t.Fatalf("retire current parent = receipt %+v err=%v", receipt, err)
	}
	replacement, _ := rawBurst(ctx, endpoint, config, []int{0})
	if len(replacement) != 1 || replacement[0].class != "ok" {
		t.Fatalf("replacement public decoded media = %+v", replacement)
	}
	newGeneration, err := target.CurrentParent(ctx, ParentFaultRequest{BaseURL: target.BaseURL, ChannelID: channel.ID})
	if err != nil || newGeneration == generation {
		t.Fatalf("replacement generation = %d (old %d), err=%v", newGeneration, generation, err)
	}
	// Keep an admitted replacement reader open through stale refusal: a new
	// tune after the refusal could otherwise hide a wrongly killed replacement.
	held := startHeldBurst(ctx, endpoint, config, []int{0})
	if len(held.results) != 1 || held.results[0].class != "ok" {
		held.release()
		t.Fatalf("replacement held media = %+v", held.results)
	}
	if _, err := target.FailParent(ctx, request); err == nil {
		held.release()
		t.Fatal("stale parent request retired replacement")
	}
	held.verify(ctx)
	continuingGeneration, generationErr := target.CurrentParent(ctx, ParentFaultRequest{BaseURL: target.BaseURL, ChannelID: channel.ID})
	held.release()
	if held.results[0].class != "ok" || generationErr != nil || continuingGeneration != newGeneration {
		t.Fatalf("replacement was affected by stale refusal: held=%+v generation=%d err=%v", held.results[0], continuingGeneration, generationErr)
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
