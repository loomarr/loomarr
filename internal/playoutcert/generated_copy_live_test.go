//go:build ffmpeg

package playoutcert_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/app"
	"github.com/loomarr/loomarr/internal/playoutcert"
)

func requireGeneratedCopyCoverage(t *testing.T, report playoutcert.Report) {
	t.Helper()
	phase := report.PhaseMust("copy_raw")
	if phase.Attempts != 1 || phase.Failures != 0 || len(phase.Media) != 1 || phase.Media[0].VideoCodec != "h264" || phase.Media[0].AudioCodec != "aac" || len(phase.HeldContinuity) != 1 {
		t.Fatalf("generated copy workload: %+v", phase)
	}
	held := phase.HeldContinuity[0]
	if held.Outcome != "observed" || !held.DecodedFrame || held.AdvancingReads == 0 || held.BytesObserved == 0 {
		t.Fatalf("generated copy did not remain live: %+v", held)
	}
	heldSamples := 0
	for _, sample := range report.Resources {
		if sample.Point == "copy_raw_held" {
			heldSamples++
			if sample.TranscodeCost != 0 || sample.ViewerActive != 1 || sample.SessionsActive != 1 {
				t.Fatalf("generated copy admission: %+v", sample)
			}
		}
	}
	if heldSamples != 1 {
		t.Fatalf("generated copy held measurements = %d", heldSamples)
	}
}

func TestGeneratedCopyReportRequiresActualMediaAndAdmission(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	for _, duration := range []time.Duration{6 * time.Second, 30 * time.Second} {
		t.Run(duration.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			channels := []playoutcert.Channel{{ID: "prepared-private", Roles: []string{"prepared"}}, {ID: "copy-private", Roles: []string{"copy", "audio_aac"}}}
			target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: 100 * time.Millisecond, ProgrammeDuration: duration})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				closeCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
				defer stop()
				if err := target.Close(closeCtx); err != nil {
					t.Error(err)
				}
			}()
			report, err := playoutcert.Run(ctx, playoutcert.Config{BaseURL: target.BaseURL, AdminBearer: target.AdminBearer, DeviceToken: target.DeviceToken, Channels: channels,
				Concurrency: 1, FanInViewers: 1, RequestTimeout: 25 * time.Second, CleanupTimeout: 8 * time.Second, CleanupPoll: 25 * time.Millisecond, WarmGrace: 100 * time.Millisecond,
				RawCaptureBytes: 2 << 20, Validator: playoutcert.FFprobeValidator{}, Decoder: playoutcert.FFmpegDecoder{}, ProgrammeEvidence: target.ProgrammeEvidence(),
				ProgrammeBoundaryTimeout: 2 * time.Second, ProgrammeBoundaryLateObservation: 250 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}
			requireGeneratedCopyCoverage(t, report)
			if phase := report.PhaseMust("configured"); phase.Attempts != 2 || phase.PreparedHits != 1 || phase.HTTPClasses["prepared_miss"] != 1 || phase.Failures != 0 {
				t.Fatalf("generated catalog evidence: %+v", phase)
			}
			if report.PhaseMust("raw_capacity").HTTPClasses["transcode_cohort_insufficient"] == 0 || report.PhaseMust("programme_boundary").Failures == 0 {
				t.Fatal("copy-only coverage certified missing capacity or programme-boundary evidence")
			}
			if report.PhaseMust("cleanup").Failures != 0 {
				t.Fatal("generated copy resources did not converge")
			}
			publication, err := playoutcert.FinalizePublication(report)
			if err != nil || publication.AuditStatus() != playoutcert.AuditPassed || publication.Verdict() == playoutcert.VerdictCertified {
				t.Fatalf("generated copy publication: %v / %s", err, publication.JSON())
			}
		})
	}
}
