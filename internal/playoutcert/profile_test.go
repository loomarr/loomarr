package playoutcert

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestDeclaredProfileRejectsUnsafeValuesBeforeObservation(t *testing.T) {
	valid := ProfileEvidence{QualityTier: "balanced", Encoder: "h264_vaapi", MeasuredCapacity: 12,
		Probe:    ProfileDimensions{Width: 1920, Height: 1080, FrameRate: 25, VideoBitrateKbps: 5000, AudioBitrateKbps: 160},
		Prepared: ProfileDimensions{Width: 1920, Height: 1080, FrameRate: 25, VideoBitrateKbps: 5000, AudioBitrateKbps: 160}}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ProfileEvidence){
		func(p *ProfileEvidence) { p.Encoder = "secret-encoder" },
		func(p *ProfileEvidence) { p.QualityTier = "https://private.invalid" },
		func(p *ProfileEvidence) { p.MeasuredCapacity = 65 },
		func(p *ProfileEvidence) { p.Probe.Width = 0 },
		func(p *ProfileEvidence) { p.Prepared.FrameRate = 1000 },
	} {
		profile := valid
		mutate(&profile)
		_, err := Run(context.Background(), Config{Profile: &profile})
		if err == nil || !strings.HasPrefix(err.Error(), "invalid declared target") {
			t.Fatalf("profile rejected after target observation: %v", err)
		}
	}
}

func TestDeclaredProfileIsFrozenAndAuditedWithRun(t *testing.T) {
	fixture := playoutcertfixture.New(t, 100)
	// The usual exp=1 fixture intentionally collides with ordinary report
	// counters; use a real-sized expiry when testing successful publication.
	fixture.Expiry = "9876543210"
	profile := &ProfileEvidence{QualityTier: "balanced", Encoder: "h264_vaapi", MeasuredCapacity: 4,
		Probe:    ProfileDimensions{Width: 1920, Height: 1080, FrameRate: 25, VideoBitrateKbps: 5000, AudioBitrateKbps: 160},
		Prepared: ProfileDimensions{Width: 1920, Height: 1080, FrameRate: 25, VideoBitrateKbps: 5000, AudioBitrateKbps: 160}}
	report, err := Run(t.Context(), Config{
		BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, DeviceToken: fixture.Device, Channels: fixtureChannels(100),
		Profile: profile, Concurrency: 12, FanInViewers: 4, RequestTimeout: time.Second,
		CleanupTimeout: time.Second, CleanupPoll: time.Millisecond, RawCaptureBytes: 188,
		Validator: &playoutcertfixture.ShapeValidator[MediaShape]{Shapes: []MediaShape{{VideoStreams: 1, AudioStreams: 1, VideoCodec: "h264", AudioCodec: "aac"}}},
		Decoder:   &playoutcertfixture.Decoder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile.Encoder = "private-value-after-run"
	if report.Target.Profile == nil || report.Target.Profile.Encoder != "h264_vaapi" {
		t.Fatal("profile evidence aliases caller state")
	}
	publication, err := FinalizePublication(report)
	if err != nil || publication.AuditStatus() != AuditPassed || !strings.Contains(string(publication.JSON()), `"qualityTier":"balanced"`) {
		var result struct {
			AuditReason string `json:"auditReason"`
		}
		_ = json.Unmarshal(publication.JSON(), &result)
		t.Fatalf("profile publication was not audited: status=%s reason=%s err=%v", publication.AuditStatus(), result.AuditReason, err)
	}
}
