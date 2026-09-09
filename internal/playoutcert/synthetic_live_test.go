//go:build ffmpeg

package playoutcert_test

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/app"
	"github.com/loomarr/loomarr/internal/playoutcert"
	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestSyntheticTargetCertifiesHundredPreparedChannelsAndBoundedTranscodeBurst(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := make([]playoutcert.Channel, 0, 105)
	for index := range 100 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("prepared-%03d", index+1), Roles: []string{"prepared"}})
	}
	for index := range 5 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("transcode-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	channels = append(channels, playoutcert.Channel{ID: "copy-private", Roles: []string{"copy", "audio_aac"}})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 4, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
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
	config := playoutcert.Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Channels: channels, Certify: true, Concurrency: 12, SurfRounds: 1, FanInViewers: 4,
		RequestTimeout: 15 * time.Second, CleanupTimeout: 10 * time.Second, CleanupPoll: 25 * time.Millisecond,
		WarmGrace:       time.Second,
		RawCaptureBytes: 2 << 20, PreparedP95: 100 * time.Millisecond,
		ProgrammeBoundaryTimeout: 15 * time.Second, ProgrammeBoundaryLateObservation: time.Second,
		ProgrammeEvidence: target.ProgrammeEvidence(),
		FaultProfiles:     []playoutcert.FaultProfile{playoutcert.FaultParentFailure},
		FaultController:   target,
		Validator:         playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{},
	}
	report, err := playoutcert.Run(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	requireGeneratedCopyCoverage(t, report)
	parentFault := report.PhaseMust("parent_failure")
	if parentFault.Failures != 0 || parentFault.Attempts != 1 || len(parentFault.Media) != 1 {
		t.Fatalf("parent-failure recovery evidence = %+v", parentFault)
	}
	for _, row := range report.FaultProfiles {
		if row.Profile == playoutcert.FaultParentFailure {
			if row.Status != "qualified" || row.Outcome != "complete" || row.Baseline == nil || row.PhasePeak == nil || row.Final == nil || row.ReceiptOutcome != "exited" || row.SelectedContinuity != "interrupted" || row.PeerContinuity != "continued" || row.Recovery != "recovered" {
				t.Fatalf("parent-failure qualification = %+v", row)
			}
		}
	}
	if report.PhaseMust("configured").PreparedHits != 100 {
		t.Fatalf("prepared hits = %d", report.PhaseMust("configured").PreparedHits)
	}
	if configured := report.PhaseMust("configured"); configured.Attempts != len(channels) || configured.HTTPClasses["prepared_miss"] != 6 {
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
	playoutcert.RequireCertifiedPublicationForTest(t, report)
}

func TestSyntheticTargetShutdownCertifiesMeasuredLiveBurst(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	// Prepared channels are an explicit control cohort, never substitutes for
	// the live program encoders which must exist at shutdown.
	channels := make([]playoutcert.Channel, 0, 105)
	for index := range 100 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("prepared-control-%03d", index+1), Roles: []string{"prepared"}})
	}
	// The excess channel is required by raw_capacity and overload; the shutdown
	// drill itself still holds exactly the four admitted viewers.
	for index := range 5 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("shutdown-live-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	channels = append(channels, playoutcert.Channel{ID: "copy-private", Roles: []string{"copy", "audio_aac"}})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 4, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
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
	report, err := playoutcert.Run(ctx, playoutcert.Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Channels: channels, Certify: true, DisposableTarget: target.Scope(), Concurrency: 12, SurfRounds: 1, FanInViewers: 4,
		RequestTimeout: 15 * time.Second, CleanupTimeout: 10 * time.Second, CleanupPoll: 25 * time.Millisecond,
		WarmGrace: time.Second, RawCaptureBytes: 2 << 20, PreparedP95: 100 * time.Millisecond,
		ProgrammeBoundaryTimeout: 15 * time.Second, ProgrammeBoundaryLateObservation: time.Second,
		ProgrammeEvidence: target.ProgrammeEvidence(), FaultProfiles: []playoutcert.FaultProfile{playoutcert.FaultShutdown}, FaultController: target,
		Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireGeneratedCopyCoverage(t, report)
	shutdown := report.PhaseMust("shutdown")
	if shutdown.Failures != 0 || shutdown.Resources.Samples != 1 || shutdown.Resources.Maximum.TranscodeCost < 4 || shutdown.Resources.Maximum.ViewerActive < 4 {
		t.Fatalf("shutdown live burst evidence = %+v", shutdown)
	}
	for _, row := range report.FaultProfiles {
		if row.Profile == playoutcert.FaultShutdown && (row.Status != "qualified" || row.Outcome != "complete" || row.Baseline == nil || row.PhasePeak == nil || row.Final == nil || row.ReceiptOutcome != "exited" || row.SelectedContinuity != "interrupted") {
			t.Fatalf("shutdown qualification = %+v", row)
		}
	}
	if _, err := http.Get(target.BaseURL + "/v1/playout/status"); err == nil {
		t.Fatal("public listener remained available after shutdown")
	}
	playoutcert.RequireCertifiedPublicationForTest(t, report)
}

func TestSyntheticTargetShutdownFailureCannotRetainQualification(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := make([]playoutcert.Channel, 0, 102)
	for index := range 100 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("shutdown-control-%03d", index+1), Roles: []string{"prepared"}})
	}
	for index := range 2 { // capacity one plus the required overload channel
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("shutdown-failure-live-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
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
	report, err := playoutcert.Run(ctx, playoutcert.Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Channels: channels, Certify: true, DisposableTarget: target.Scope(), Concurrency: 12, SurfRounds: 1, FanInViewers: 1,
		RequestTimeout: 15 * time.Second, CleanupTimeout: 10 * time.Second, CleanupPoll: 25 * time.Millisecond,
		WarmGrace: time.Second, RawCaptureBytes: 2 << 20, PreparedP95: time.Nanosecond,
		ProgrammeBoundaryTimeout: 15 * time.Second, ProgrammeBoundaryLateObservation: time.Second,
		ProgrammeEvidence: target.ProgrammeEvidence(), FaultProfiles: []playoutcert.FaultProfile{playoutcert.FaultShutdown}, FaultController: target,
		Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(report.Failures, ","), "prepared_p95_exceeded") {
		t.Fatalf("required phase failure did not fail the run: failures=%v", report.Failures)
	}
	for _, row := range report.FaultProfiles {
		if row.Profile == playoutcert.FaultShutdown && (row.Status != "unavailable" || row.Outcome != "run_failed" || row.Baseline == nil || row.PhasePeak == nil || row.Final == nil || row.ReceiptOutcome != "exited" || row.SelectedContinuity != "interrupted") {
			t.Fatalf("failed run retained shutdown qualification or lost evidence: %+v", row)
		}
	}
	playoutcert.RequireUncertifiedPublicationForTest(t, report)
}

// This is deliberately independent of SyntheticTarget's raw BlockSource
// witness.  It proves the ordinary public prepared route can itself carry a
// decoder across the renderer's discontinuity/map/PDT programme boundary.
func TestSyntheticPreparedPublicHLSChecksProgrammeSignals(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := []playoutcert.Channel{{ID: "prepared-boundary", Roles: []string{"prepared"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 4 * time.Second})
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
	config := playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels, ProgrammeEvidence: target.ProgrammeEvidence(), RequestTimeout: 10 * time.Second, RawCaptureBytes: 256 << 10, ProgrammeBoundaryTimeout: 20 * time.Second, ProgrammeBoundaryLateObservation: 3 * time.Second, Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{}}
	endpoint, err := playoutcert.NewEndpointForTest(playoutcert.NormalizeConfigForTest(config))
	if err != nil {
		t.Fatal(err)
	}
	result := playoutcert.ObserveProgrammeBoundaryForTest(ctx, endpoint, playoutcert.NormalizeConfigForTest(config), "prepared", 0)
	if result.Class != "ok" || result.Evidence.Transitions != 1 || result.Evidence.DecodedFrameDelta <= 0 || result.Evidence.ReadDelta <= 0 {
		t.Fatalf("prepared public boundary=%+v", result)
	}
}

func TestSyntheticPreparedPublicHLSContinuesAcrossFurtherProgrammeEpochs(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := []playoutcert.Channel{{ID: "prepared-continuous-epochs", Roles: []string{"prepared"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 4 * time.Second})
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
	config := playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels, ProgrammeEvidence: target.ProgrammeEvidence(), RequestTimeout: 10 * time.Second, RawCaptureBytes: 256 << 10, ProgrammeBoundaryTimeout: 20 * time.Second, ProgrammeBoundaryLateObservation: 6 * time.Second, Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{}}
	endpoint, err := playoutcert.NewEndpointForTest(playoutcert.NormalizeConfigForTest(config))
	if err != nil {
		t.Fatal(err)
	}
	result := playoutcert.ObserveProgrammeBoundaryForTest(ctx, endpoint, playoutcert.NormalizeConfigForTest(config), "prepared", 0)
	if result.Class != "ok" || result.Evidence.Transitions < 2 || result.Evidence.DecodedFrameDelta <= 0 || result.Evidence.ReadDelta <= 0 || result.Evidence.BytesDelta <= 0 {
		t.Fatalf("prepared public continuous epochs=%+v", result)
	}
}

func TestSyntheticTargetParentFaultCertifiesAtOneMeasuredSlot(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := make([]playoutcert.Channel, 0, 101)
	for index := range 99 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("prepared-%03d", index+1), Roles: []string{"prepared"}})
	}
	for index := range 2 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("transcode-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	channels = append(channels, playoutcert.Channel{ID: "copy-private", Roles: []string{"copy", "audio_aac"}})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
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
	report, err := playoutcert.Run(ctx, playoutcert.Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Channels: channels, Certify: true, Concurrency: 12, SurfRounds: 1, FanInViewers: 1,
		RequestTimeout: 15 * time.Second, CleanupTimeout: 10 * time.Second, CleanupPoll: 25 * time.Millisecond,
		WarmGrace: time.Second, RawCaptureBytes: 2 << 20, PreparedP95: 100 * time.Millisecond,
		ProgrammeBoundaryTimeout: 15 * time.Second, ProgrammeBoundaryLateObservation: time.Second,
		ProgrammeEvidence: target.ProgrammeEvidence(), FaultProfiles: []playoutcert.FaultProfile{playoutcert.FaultParentFailure}, FaultController: target,
		Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireGeneratedCopyCoverage(t, report)
	if strings.Contains(strings.Join(report.Failures, ","), "capacity_oversubscribed") {
		t.Fatalf("one-slot run oversubscribed capacity: failures %v resources=%+v", report.Failures, report.Resources)
	}
	for _, row := range report.FaultProfiles {
		if row.Profile == playoutcert.FaultParentFailure && (row.Status != "qualified" || row.PeerContinuity != "not_applicable" || row.SelectedContinuity != "interrupted" || row.Recovery != "recovered") {
			t.Fatalf("one-slot parent evidence = %+v", row)
		}
	}
	playoutcert.RequireCertifiedPublicationForTest(t, report)
}

func TestSyntheticTargetRejectsStaleParentGenerationAfterReplacement(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	channel := playoutcert.Channel{ID: "transcode", Roles: []string{"transcode_h264", "audio_aac"}}
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: []playoutcert.Channel{channel}, FFmpeg: ffmpeg, Capacity: 1, Grace: time.Second})
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
	config := playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: []playoutcert.Channel{channel}, RequestTimeout: 15 * time.Second, RawCaptureBytes: 2 << 20, Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{}}
	config = playoutcert.NormalizeConfigForTest(config)
	endpoint, err := playoutcert.NewEndpointForTest(config)
	if err != nil {
		t.Fatal(err)
	}
	first, sample := playoutcert.RawBurstForTest(ctx, endpoint, config, []int{0})
	if len(first) != 1 || first[0].Class != "ok" || sample.Capacity == 0 {
		t.Fatalf("initial public media = observations %+v sample=%+v", first, sample)
	}
	request := playoutcert.ParentFaultRequest{BaseURL: target.BaseURL, ChannelID: channel.ID}
	generation, err := target.CurrentParent(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	request.Generation = generation
	if receipt, err := target.FailParent(ctx, request); err != nil || !receipt.Exited {
		t.Fatalf("retire current parent = receipt %+v err=%v", receipt, err)
	}
	replacement, _ := playoutcert.RawBurstForTest(ctx, endpoint, config, []int{0})
	if len(replacement) != 1 || replacement[0].Class != "ok" {
		t.Fatalf("replacement public decoded media = %+v", replacement)
	}
	newGeneration, err := target.CurrentParent(ctx, playoutcert.ParentFaultRequest{BaseURL: target.BaseURL, ChannelID: channel.ID})
	if err != nil || newGeneration == generation {
		t.Fatalf("replacement generation = %d (old %d), err=%v", newGeneration, generation, err)
	}
	// Keep an admitted replacement reader open through stale refusal: a new
	// tune after the refusal could otherwise hide a wrongly killed replacement.
	held := playoutcert.StartHeldBurstForTest(ctx, endpoint, config, []int{0})
	if len(held.Results()) != 1 || held.Results()[0].Class != "ok" {
		held.Release()
		t.Fatalf("replacement held media = %+v", held.Results())
	}
	if _, err := target.FailParent(ctx, request); err == nil {
		held.Release()
		t.Fatal("stale parent request retired replacement")
	}
	held.Verify(ctx)
	continuingGeneration, generationErr := target.CurrentParent(ctx, playoutcert.ParentFaultRequest{BaseURL: target.BaseURL, ChannelID: channel.ID})
	held.Release()
	if held.Results()[0].Class != "ok" || generationErr != nil || continuingGeneration != newGeneration {
		t.Fatalf("replacement was affected by stale refusal: held=%+v generation=%d err=%v", held.Results()[0], continuingGeneration, generationErr)
	}
}

func TestSyntheticTargetChildFaultDrillExitsOwnedEncoderAndRecoversPeer(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := []playoutcert.Channel{
		{ID: "transcode-selected", Roles: []string{"transcode_h264", "audio_aac"}},
		{ID: "transcode-peer", Roles: []string{"transcode_h264", "audio_aac"}},
		// Marking a separate prepared control prevents app.NewPlayoutCertificationTarget from
		// treating the two transcode channels as the implicit prepared cohort.
		{ID: "prepared-control", Roles: []string{"prepared"}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 2, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
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
	config := playoutcert.Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels,
		RequestTimeout: 15 * time.Second, RawCaptureBytes: 2 << 20, CleanupPoll: 25 * time.Millisecond,
		Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{}, FaultController: target,
	}
	config = playoutcert.NormalizeConfigForTest(config)
	endpoint, err := playoutcert.NewEndpointForTest(config)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the selected and peer viewers attached while waiting for a finite child.
	// A raw burst releases them before returning and can outlive the encoder.
	initialViewers := playoutcert.StartHeldBurstForTest(ctx, endpoint, config, []int{0, 1})
	initialReleased := false
	defer func() {
		if !initialReleased {
			initialViewers.Release()
		}
	}()
	initial := initialViewers.Results()
	initialSample, sampleErr := playoutcert.SampleResources(ctx, config, "raw_capacity")
	if sampleErr != nil {
		t.Fatal(sampleErr)
	}
	if len(initial) != 2 || initial[0].Class != "ok" || initial[1].Class != "ok" || initialSample.PreparedChannels != 1 || initialSample.TranscodeCost < 2 {
		t.Fatalf("selected/peer did not enter the live transcode cohort: observations=%+v sample=%+v", initial, initialSample)
	}
	childCtx, cancelChild := context.WithTimeout(ctx, config.RequestTimeout)
	defer cancelChild()
	stale, err := playoutcert.WaitCurrentChildForTest(childCtx, target, playoutcert.ChildFaultRequest{BaseURL: target.BaseURL, ChannelID: channels[0].ID}, config.CleanupPoll)
	if err != nil {
		t.Fatalf("current selected live child before drill: %v", err)
	}
	if _, err := target.CurrentChild(ctx, playoutcert.ChildFaultRequest{BaseURL: target.BaseURL + "/wrong", ChannelID: channels[0].ID}); err == nil {
		t.Fatal("mismatched child target exposed an owned encoder")
	}
	cancelled, cancelCurrent := context.WithCancel(ctx)
	cancelCurrent()
	if _, err := target.CurrentChild(cancelled, playoutcert.ChildFaultRequest{BaseURL: target.BaseURL, ChannelID: channels[0].ID}); err == nil {
		t.Fatal("cancelled child lookup exposed an owned encoder")
	}
	if _, err := target.FailChild(ctx, playoutcert.ChildFaultRequest{BaseURL: target.BaseURL, ChannelID: channels[0].ID, ParentGeneration: stale.ParentGeneration + 1, ChildGeneration: stale.ChildGeneration}); err == nil {
		t.Fatal("reused parent generation signalled the current child")
	}
	initialViewers.Release()
	initialReleased = true
	drill := playoutcert.ChildFailureDrillForTest(ctx, endpoint, config, []int{0, 1}, 2)
	if drill.Phase.Failures != 0 || drill.Receipt != "exited" || drill.Peer != "continued" || drill.Recovery != "recovered" {
		t.Fatalf("child fault drill did not prove owned exit and peer recovery: %+v", drill)
	}
	if drill.Selected == "" || drill.Selected == "not_observed" {
		t.Fatalf("child fault drill did not record selected stream outcome: %+v", drill)
	}
	replacement, replacementSample := playoutcert.RawBurstForTest(ctx, endpoint, config, []int{0})
	if len(replacement) != 1 || replacement[0].Class != "ok" || replacementSample.TranscodeCost < 1 {
		t.Fatalf("selected public replacement was not live media: observations=%+v sample=%+v", replacement, replacementSample)
	}
	held := playoutcert.StartHeldBurstForTest(ctx, endpoint, config, []int{0})
	if len(held.Results()) != 1 || held.Results()[0].Class != "ok" {
		held.Release()
		t.Fatalf("replacement held media = %+v", held.Results())
	}
	if _, err := target.FailChild(ctx, playoutcert.ChildFaultRequest{BaseURL: target.BaseURL, ChannelID: channels[0].ID, ParentGeneration: stale.ParentGeneration, ChildGeneration: stale.ChildGeneration}); err == nil {
		held.Release()
		t.Fatal("stale child request signalled the replacement")
	}
	held.Verify(ctx)
	held.Release()
	if held.Results()[0].Class != "ok" {
		t.Fatalf("replacement was affected by stale child refusal: %+v", held.Results()[0])
	}
}

func TestSyntheticTargetRejectsPostOverloadCorruptHeldMedia(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	channels := make([]playoutcert.Channel, 0, 105)
	for index := range 100 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("prepared-%03d", index+1), Roles: []string{"prepared"}})
	}
	for index := range 5 {
		channels = append(channels, playoutcert.Channel{ID: fmt.Sprintf("transcode-%03d", index+1), Roles: []string{"transcode_h264", "audio_aac"}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	heldStreams := make([]string, 4)
	for index := range heldStreams {
		heldStreams[index] = fmt.Sprintf("transcode-%03d", index+1)
	}
	target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 4, Grace: time.Second, ProgrammeDuration: 6 * time.Second})
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
	config := playoutcert.Config{
		BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken,
		Channels: channels, Certify: true, Concurrency: 12, SurfRounds: 1, FanInViewers: 4,
		RequestTimeout: 15 * time.Second, CleanupTimeout: 10 * time.Second, CleanupPoll: 25 * time.Millisecond,
		WarmGrace:       time.Second,
		RawCaptureBytes: 2 << 20, PreparedP95: 100 * time.Millisecond,
		ProgrammeBoundaryTimeout: 15 * time.Second, ProgrammeBoundaryLateObservation: time.Second,
		ProgrammeEvidence: target.ProgrammeEvidence(),
		Validator:         playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{},
		Client: playoutcertfixture.PostEventCorruptClient(http.DefaultTransport.(*http.Transport).Clone(), heldStreams),
	}
	report, err := playoutcert.Run(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	rawCapacity := report.PhaseMust("raw_capacity")
	if rawCapacity.Failures != 0 || len(rawCapacity.Media) != 4 {
		t.Fatalf("initial raw-capacity media was not valid: %+v", rawCapacity)
	}
	overload := report.PhaseMust("overload")
	if overload.HTTPClasses["http_503"] != 1 || len(overload.HeldContinuity) != 4 {
		t.Fatalf("corrupt post-event run omitted failure evidence: %+v", overload)
	}
	for _, held := range overload.HeldContinuity {
		if held.Outcome == "observed" || held.DecodedFrame || held.BytesObserved == 0 {
			t.Fatalf("corrupt post-event media passed: %+v", held)
		}
	}
	playoutcert.RequireUncertifiedPublicationForTest(t, report)
}
