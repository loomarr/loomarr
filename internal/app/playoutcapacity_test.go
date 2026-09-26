package app

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/playout"
)

// G11: a tone-map that fails is a red, immediate Current Health error, never silent; one that works
// too slowly for a live channel warns; a working one passes and names its stage.
func TestTonemapObservation_FailureIsRedAndImmediate(t *testing.T) {
	startup := diagnostics.NewStartup(time.Now(), 1, "v1", []diagnostics.StartupCheck{
		{Key: diagnostics.StartupCheckPlayoutTonemap, Label: "HDR tone mapping", Mode: diagnostics.HealthCheckContinuous},
	}, time.Now)
	startup.Complete(diagnostics.StartupCheckPlayoutTonemap, diagnostics.StartupSkipped, "pending probe", "", "")

	cases := []struct {
		check playout.TonemapCheck
		want  diagnostics.HealthCheckStatus
	}{
		{playout.TonemapCheck{Ran: true, Detail: "tonemap_opencl: no OpenCL platform"}, diagnostics.HealthFailed},
		{playout.TonemapCheck{Ran: true, OK: true, Stage: "cpu", Detail: "runs at 0.90x"}, diagnostics.HealthWarning},
		{playout.TonemapCheck{Ran: true, OK: true, Stage: "tonemap_vaapi"}, diagnostics.HealthPassed},
	}
	for _, tc := range cases {
		obs, ok := tonemapObservation(tc.check)
		if !ok {
			t.Fatalf("%+v produced no observation", tc.check)
		}
		startup.Observe(diagnostics.StartupCheckPlayoutTonemap, obs)
		got := startup.Health().Checks[0]
		if got.Status != tc.want {
			t.Errorf("%+v: health %s, want %s (%s)", tc.check, got.Status, tc.want, got.Detail)
		}
	}
	if _, ok := tonemapObservation(playout.TonemapCheck{}); ok {
		t.Error("a check that never ran reported health")
	}
}
