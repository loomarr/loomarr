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

func TestCertificationTargetRejectsMislabeledSourceCodecs(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	for _, mode := range []string{"operator", "generated"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			channels := []playoutcert.Channel{{ID: "copy-private", Roles: []string{"copy", "audio_eac3"}}, {ID: "prepared-private", Roles: []string{"prepared"}}}
			var cohort *playoutcert.OperatorCohort
			if mode == "operator" {
				var manifest string
				manifest, channels = operatorCapacityCorpus(t, ctx, ffmpeg, t.TempDir(), []operatorMediaProfile{
					{"copy-private", "h264", "aac", 320, 180, 1, []string{"copy", "audio_eac3"}},
					// A real EAC3 source elsewhere must not satisfy the AAC Channel's label.
					{"eac3-peer", "h264", "eac3", 320, 180, 1, []string{"copy", "audio_eac3"}},
				})
				cohort, err = playoutcert.LoadOperatorCohort(ctx, manifest, channels)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = cohort.Close() }()
			}
			target, err := app.NewPlayoutCertificationTarget(ctx, app.PlayoutCertificationConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 1, Grace: 100 * time.Millisecond, ProgrammeDuration: 6 * time.Second, Cohort: cohort})
			if target != nil {
				closeCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
				defer stop()
				_ = target.Close(closeCtx)
				t.Fatal("mislabeled source started a certification target")
			}
			if err == nil || err.Error() != "certification source codec does not match declared role" {
				t.Fatalf("source codec mismatch not rejected with fixed error: %v", err)
			}
		})
	}
}
