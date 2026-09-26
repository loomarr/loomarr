package playoutbench

import (
	"path/filepath"
	"strings"
	"testing"
)

func report(family string, set func(*Report)) *Report {
	r := NewReport(family, "abc123", "generated-v1", Host{OS: "linux", Arch: "amd64", CPUs: 4})
	set(r)
	return r
}

func statuses(t *testing.T, deltas []Delta) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, d := range deltas {
		out[d.Metric] = d.Status
	}
	return out
}

func TestCompare_ToleranceAndDirection(t *testing.T) {
	base := report("vaapi", func(r *Report) {
		r.Set("start_p95_ms/h264-1080p", 300, "ms", Lower)
		r.Set("speed_x/h264-1080p", 18, "x", Higher)
		r.Set("break/pts_gaps", 0, "count", Exact)
		r.Set("cores_per_stream/h264-1080p", 0.05, "cores", Lower)
	})
	cur := report("vaapi", func(r *Report) {
		r.Set("start_p95_ms/h264-1080p", 340, "ms", Lower) // +13%: inside 15%
		r.Set("speed_x/h264-1080p", 14, "x", Higher)       // -22%: regression
		r.Set("break/pts_gaps", 1, "count", Exact)         // any change
		r.Set("cores_per_stream/h264-1080p", 0.049, "cores", Lower)
	})
	deltas, regressed, err := Compare(base, cur, DefaultTolerance())
	if err != nil {
		t.Fatal(err)
	}
	if !regressed {
		t.Fatal("a 22% speed loss and a new PTS gap must regress")
	}
	got := statuses(t, deltas)
	want := map[string]string{
		"start_p95_ms/h264-1080p":     StatusOK,
		"speed_x/h264-1080p":          StatusRegression,
		"break/pts_gaps":              StatusRegression,
		"cores_per_stream/h264-1080p": StatusImproved,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q, want %q", k, got[k], w)
		}
	}
}

func TestCompare_SlackAbsorbsNoiseOnSmallValues(t *testing.T) {
	// 40 ms -> 55 ms is +37% but 15 ms is inside the 25 ms slack: a fast start is not a regression
	// because the timer moved by a scheduler tick.
	base := report("nvenc", func(r *Report) { r.Set("start_p95_ms/x", 40, "ms", Lower) })
	cur := report("nvenc", func(r *Report) { r.Set("start_p95_ms/x", 55, "ms", Lower) })
	_, regressed, err := Compare(base, cur, DefaultTolerance())
	if err != nil || regressed {
		t.Fatalf("regressed=%v err=%v, want neither", regressed, err)
	}
}

func TestCompare_MissingSkippedAndNew(t *testing.T) {
	base := report("software", func(r *Report) {
		r.Set("vmaf_mean/h264-1080p", 92, "vmaf", Higher)
		r.Set("speed_x/h264-1080p", 3, "x", Higher)
	})
	cur := report("software", func(r *Report) {
		r.Skipped["vmaf_mean/h264-1080p"] = "ffmpeg has no libvmaf"
		r.Set("speed_x/hevc-1080p", 2, "x", Higher)
	})
	deltas, regressed, err := Compare(base, cur, DefaultTolerance())
	if err != nil {
		t.Fatal(err)
	}
	got := statuses(t, deltas)
	if got["vmaf_mean/h264-1080p"] != StatusSkipped {
		t.Errorf("a metric the host declared skipped is a warning, got %q", got["vmaf_mean/h264-1080p"])
	}
	if got["speed_x/h264-1080p"] != StatusMissing || !regressed {
		t.Errorf("a metric that vanished without a skip reason must regress, got %q regressed=%v", got["speed_x/h264-1080p"], regressed)
	}
	if got["speed_x/hevc-1080p"] != StatusNew {
		t.Errorf("new metric = %q", got["speed_x/hevc-1080p"])
	}
}

func TestCompare_RefusesAcrossFamilies(t *testing.T) {
	a := report("vaapi", func(*Report) {})
	b := report("nvenc", func(*Report) {})
	if _, _, err := Compare(a, b, DefaultTolerance()); err == nil {
		t.Fatal("comparing vaapi with nvenc numbers is meaningless and must be an error")
	}
}

func hardwareGreen(r *Report) {
	r.Set("start_p95_ms/h264-1080p", 350, "ms", Lower)
	r.Set("start_p95_ms/hevc-1080p", 300, "ms", Lower)
	r.Set("start_p95_ms/hdr-4k", 1200, "ms", Lower)
	r.Set("speed_x/h264-1080p", 17, "x", Higher)
	r.Set("speed_x/hevc-1080p", 18, "x", Higher)
	r.Set("speed_x/hdr-4k", 2.5, "x", Higher)
	r.Set("concurrency/max_streams", 16, "streams", Higher)
	r.Set("concurrency/total_cores", 0.9, "cores", Lower)
	r.Set("break/first_packet_ms", 60, "ms", Lower)
	r.Set("break/pts_gaps", 0, "count", Exact)
	r.Set("break/audio_off_grid", 0, "count", Exact)
	r.Set("break/sps_variants", 1, "count", Exact)
	r.Set("break/loudness_dev_lu", 0.2, "LU", Lower)
}

func failed(vs []Verdict) []string {
	var out []string
	for _, v := range vs {
		if !v.Pass {
			out = append(out, v.Metric)
		}
	}
	return out
}

func TestJudge_HardwareThresholds(t *testing.T) {
	green := report("vaapi", hardwareGreen)
	if f := failed(Judge(green, ModeFull)); len(f) != 0 {
		t.Fatalf("the phase 0 Arc numbers must pass, failed: %v", f)
	}

	red := report("vaapi", func(r *Report) {
		hardwareGreen(r)
		r.Set("start_p95_ms/h264-1080p", 496, "ms", Lower) // the 2 s segment miss
		r.Set("concurrency/total_cores", 1.41, "cores", Lower)
		r.Set("break/sps_variants", 2, "count", Exact)
	})
	f := failed(Judge(red, ModeFull))
	for _, want := range []string{"start_p95_ms/h264-1080p", "concurrency/total_cores", "break/sps_variants"} {
		if !contains(f, want) {
			t.Errorf("%s should fail, failed: %v", want, f)
		}
	}
	if len(f) != 3 {
		t.Errorf("only the three regressions should fail, got %v", f)
	}
}

func TestJudge_NotMeasuredIsAFailureUnlessSkipped(t *testing.T) {
	r := report("nvenc", func(r *Report) {
		hardwareGreen(r)
		delete(r.Metrics, "speed_x/hdr-4k")
		delete(r.Metrics, "start_p95_ms/hdr-4k")
		r.Skipped["speed_x/hdr-4k"] = "libplacebo Vulkan unavailable"
	})
	f := failed(Judge(r, ModeFull))
	if contains(f, "speed_x/hdr-4k") {
		t.Errorf("a declared skip is not a failure: %v", f)
	}
	if !contains(f, "start_p95_ms/hdr-4k") {
		t.Errorf("a silently absent metric must fail: %v", f)
	}
}

func TestJudge_CorrectnessModeIgnoresPerformance(t *testing.T) {
	// A virtualised macOS runner is slow by construction; it still must not tear or drift.
	r := report("videotoolbox", func(r *Report) {
		hardwareGreen(r)
		r.Set("speed_x/h264-1080p", 6.3, "x", Higher)
		r.Set("break/pts_gaps", 3, "count", Exact)
	})
	if f := failed(Judge(r, ModeCorrectness)); len(f) != 1 || f[0] != "break/pts_gaps" {
		t.Fatalf("correctness mode should fail only the gap, got %v", f)
	}
	if f := failed(Judge(r, ModeFull)); !contains(f, "speed_x/h264-1080p") {
		t.Fatalf("full mode must judge speed, got %v", f)
	}
}

func TestJudge_SoftwareOnlyHasNoGPUThresholds(t *testing.T) {
	r := report("software", func(r *Report) {
		r.Set("concurrency/max_streams", 2, "streams", Higher)
		r.Set("break/first_packet_ms", 120, "ms", Lower)
		r.Set("break/pts_gaps", 0, "count", Exact)
		r.Set("break/audio_off_grid", 0, "count", Exact)
		r.Set("break/sps_variants", 1, "count", Exact)
		r.Set("break/loudness_dev_lu", 0.3, "LU", Lower)
	})
	if f := failed(Judge(r, ModeFull)); len(f) != 0 {
		t.Fatalf("software-only is judged on sustaining a stream and correctness, failed: %v", f)
	}
}

func TestReportRoundTripAndSchemaGuard(t *testing.T) {
	r := report("vaapi", hardwareGreen)
	path := filepath.Join(t.TempDir(), "vaapi.json")
	if err := r.Write(path); err != nil {
		t.Fatal(err)
	}
	back, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Metrics["speed_x/hevc-1080p"].Value != 18 {
		t.Fatalf("round trip lost a metric: %+v", back.Metrics["speed_x/hevc-1080p"])
	}
	back.SchemaVersion = SchemaVersion + 1
	if err := back.Write(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "re-accept") {
		t.Fatalf("a foreign schema must be refused with the remedy, got %v", err)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// withExpectedFailures swaps the package's known-failure table for one test.
func withExpectedFailures(t *testing.T, m map[string]string) {
	t.Helper()
	old := expectedFailures
	expectedFailures = m
	t.Cleanup(func() { expectedFailures = old })
}

func TestJudge_ExpectedFailureIsReportedNotHidden(t *testing.T) {
	withExpectedFailures(t, map[string]string{"break/sps_variants": "phase 2 lands setsar"})
	r := report("vaapi", hardwareGreen)
	r.Set("break/sps_variants", 2, "count", Exact)
	vs := Judge(r, ModeFull)
	if f := failed(vs); len(f) != 0 {
		t.Fatalf("a known failure must not fail the run: %v", f)
	}
	for _, v := range vs {
		if v.Metric == "break/sps_variants" && !strings.Contains(v.Measured, "expected fail: phase 2 lands setsar") {
			t.Errorf("the failure must stay visible with its reference, got %q", v.Measured)
		}
	}
}

func TestJudge_UnexpectedPassOfAKnownFailureFails(t *testing.T) {
	withExpectedFailures(t, map[string]string{"break/sps_variants": "phase 2 lands setsar"})
	r := report("vaapi", hardwareGreen) // sps_variants = 1: the fix has landed
	if f := failed(Judge(r, ModeFull)); len(f) != 1 || f[0] != "break/sps_variants" {
		t.Fatalf("a stale expected-fail marker must fail so it gets removed: %v", f)
	}
}

func TestExpectedFailuresAreRealMetrics(t *testing.T) {
	known := map[string]bool{}
	for _, th := range append(append([]threshold{}, commonThresholds...), hardwareThresholds...) {
		known[th.metric] = true
	}
	for m, why := range productionExpectedFailures {
		if !known[m] || why == "" {
			t.Errorf("expectedFailures[%q] = %q names no threshold or gives no reference", m, why)
		}
	}
}
