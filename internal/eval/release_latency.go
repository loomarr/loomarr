//go:build eval

package eval

import (
	"fmt"
	"slices"
)

// applyEndToEndLatency keeps wall-clock Suggester latency separate from provider-reported time.
func applyEndToEndLatency(assessment *CertificationAssessment, results []Result, thresholds CertificationThresholds) {
	timings := make([]int64, 0, len(results))
	var successfulMax int64
	for _, result := range results {
		if result.EndToEndLatencyNanos <= 0 {
			continue
		}
		timings = append(timings, result.EndToEndLatencyNanos)
		if result.Passed() {
			successfulMax = max(successfulMax, result.EndToEndLatencyNanos)
		}
	}
	slices.Sort(timings)
	assessment.Performance.EndToEndMeasuredRuns = len(timings)
	assessment.Performance.EndToEndLatencyP50Nanos = percentileInt64(timings, 0.50)
	assessment.Performance.EndToEndLatencyP95Nanos = percentileInt64(timings, 0.95)
	assessment.Performance.MaxSuccessfulEndToEndLatencyNanos = successfulMax
	enabled := thresholds.MaxP50EndToEndLatencyNanos > 0 || thresholds.MaxP95EndToEndLatencyNanos > 0 || thresholds.MaxSuccessfulEndToEndLatencyNanos > 0
	if enabled && (len(timings) == 0 || len(timings) != len(results)) {
		assessment.Failures = append(assessment.Failures, "end-to-end latency evidence is incomplete")
	}
	for _, gate := range []struct {
		name          string
		actual, limit int64
	}{
		{"p50", assessment.Performance.EndToEndLatencyP50Nanos, thresholds.MaxP50EndToEndLatencyNanos},
		{"p95", assessment.Performance.EndToEndLatencyP95Nanos, thresholds.MaxP95EndToEndLatencyNanos},
		{"successful maximum", successfulMax, thresholds.MaxSuccessfulEndToEndLatencyNanos},
	} {
		if gate.limit > 0 && gate.actual > gate.limit {
			assessment.Failures = append(assessment.Failures, fmt.Sprintf("end-to-end %s latency %d > %d ns", gate.name, gate.actual, gate.limit))
		}
	}
}
