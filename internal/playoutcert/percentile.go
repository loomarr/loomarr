package playoutcert

import (
	"math"
	"sort"
	"time"
)

func summarize(success []time.Duration, failures int) LatencySummary {
	values := append([]time.Duration(nil), success...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return LatencySummary{
		Attempts: len(values) + failures, Successes: len(values), Failures: failures,
		P50MS: durationMS(nearestRank(values, 0.50)),
		P95MS: durationMS(nearestRank(values, 0.95)),
		P99MS: durationMS(nearestRank(values, 0.99)),
	}
}

func nearestRank(values []time.Duration, percentile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(percentile*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func durationMS(value time.Duration) float64 {
	return float64(value.Microseconds()) / 1000
}
