package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	proposalJobOutcomes = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "proposal_job", Name: "outcomes_total",
		Help: "Terminal proposal jobs by bounded public result code.",
	}, []string{"result"})
	proposalJobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "loomarr", Subsystem: "proposal_job", Name: "duration_seconds",
		Help:    "Wall-clock duration of terminal proposal jobs by bounded public result code.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
	}, []string{"result"})
)

var proposalJobResults = map[string]struct{}{
	"success":              {},
	"no_grounded_titles":   {},
	"timed_out":            {},
	"provider_unavailable": {},
	"generation_failed":    {},
}

// ProposalJobObserved records one durably terminal proposal job. Anything outside
// the public failure-code vocabulary collapses to generation_failed, so provider
// diagnostics and identifiers can never create labels.
func ProposalJobObserved(duration time.Duration, result string) {
	if _, ok := proposalJobResults[result]; !ok {
		result = "generation_failed"
	}
	proposalJobOutcomes.WithLabelValues(result).Inc()
	proposalJobDuration.WithLabelValues(result).Observe(duration.Seconds())
}
