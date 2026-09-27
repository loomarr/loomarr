package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/testkit"
)

// The operator's playout.tone_curve reaches the resolver and, through it, the budget's HDR cell
// choice: BT.2390 must be priced at its own measured cost, not Hable's.
func TestBuild_ToneCurveSettingPicksTheHDRCell(t *testing.T) {
	t.Setenv("API_TOKEN", "tone-curve-test-token")

	st := testkit.MigratedSQLiteStore(t)
	for key, value := range map[string]string{
		"playout.backend":    "internal",
		"playout.encoder":    string(playout.EncoderSoftware),
		"playout.tone_curve": "bt2390",
	} {
		if err := st.SetSetting(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	application, err := Build(ctx, st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	r := application.playoutResolver
	if r == nil {
		t.Fatal("Build wired no playout resolver")
	}
	if got := r.ToneCurve(); got != playout.ToneCurveBT2390 {
		t.Errorf("resolver ToneCurve = %q, want the playout.tone_curve setting %q", got, playout.ToneCurveBT2390)
	}

	// Install the table the class probe would publish, with distinct Hable and BT.2390 HDR cells.
	top := playout.LadderHeights(playout.TierFor(""))[0]
	r.measuredCosts.Store(&playout.MeasuredCosts{Encoder: playout.EncoderSoftware, Costs: map[playout.CostKey]playout.ClassCost{
		{Class: playout.ClassSDR, Height: top}:                           {Speed: 20, CPUCores: 0.5},
		playout.HDRKey(playout.ClassHDR4K, top, playout.ToneCurveHable):  {Speed: 4, CPUCores: 1},
		playout.HDRKey(playout.ClassHDR4K, top, playout.ToneCurveBT2390): {Speed: 3, CPUCores: 2},
	}})

	req := httptest.NewRequest(http.MethodGet, "/v1/playout/status", nil)
	req.Header.Set("Authorization", "Bearer tone-curve-test-token")
	rec := httptest.NewRecorder()
	application.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/playout/status = %d: %s", rec.Code, rec.Body.String())
	}
	var status api.PlayoutStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if hdr := status.Budget.Classes[playout.ClassHDR4K]; hdr.Speed != 3 || hdr.CPUCores != 2 {
		t.Fatalf("budget HDR cell = %+v, want the BT.2390 cell (3x, 2 cores)", hdr)
	}
}

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
