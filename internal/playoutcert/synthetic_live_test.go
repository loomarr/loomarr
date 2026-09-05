//go:build ffmpeg

package playoutcert

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"
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
	target, err := NewSyntheticTarget(ctx, SyntheticConfig{Channels: channels, FFmpeg: ffmpeg, Capacity: 4, Grace: time.Second})
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
		Validator: FFprobeValidator{}, Decoder: FFmpegDecoder{},
	}
	report, err := Run(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Certified {
		t.Fatalf("certification failed: %v\n%sresources=%+v", report.Failures, HumanSummary(report), report.Resources)
	}
	if report.PhaseMust("configured").PreparedHits != 100 {
		t.Fatalf("prepared hits = %d", report.PhaseMust("configured").PreparedHits)
	}
	if preparedRaw := report.PhaseMust("prepared_raw"); preparedRaw.Attempts != 4 || preparedRaw.Failures != 0 || preparedRaw.P95MS > 500 {
		t.Fatalf("prepared raw phase = %+v", preparedRaw)
	}
	if fanIn := report.PhaseMust("fan_in"); fanIn.Attempts != 4 || fanIn.Failures != 0 {
		t.Fatalf("fan-in phase = %+v", fanIn)
	}
	if report.PhaseMust("overload").HTTPClasses["http_503"] == 0 {
		t.Fatalf("overload was not bounded: %+v", report.PhaseMust("overload"))
	}
}
