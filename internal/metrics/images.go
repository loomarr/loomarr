package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/mantonx/loomarr/internal/images/rustgen"
)

var (
	imageWorkerOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "image_worker", Name: "operations_total",
		Help: "Rust image worker invocations by operation kind and stable result class.",
	}, []string{"kind", "result"})
	imageWorkerDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "loomarr", Subsystem: "image_worker", Name: "duration_seconds",
		Help:    "Wall-clock duration of one Rust image worker process.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"kind"})
	imageWorkerInputBytes = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "loomarr", Subsystem: "image_worker", Name: "input_bytes",
		Help:    "Compressed source bytes read by one Rust image worker process.",
		Buckets: prometheus.ExponentialBuckets(1024, 4, 9),
	}, []string{"kind"})
	imageWorkerOutputBytes = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "loomarr", Subsystem: "image_worker", Name: "output_bytes",
		Help:    "Encoded output bytes returned by one Rust image worker process.",
		Buckets: prometheus.ExponentialBuckets(1024, 4, 9),
	}, []string{"kind"})
	imageWorkerPeakRSSBytes = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "loomarr", Subsystem: "image_worker", Name: "peak_rss_bytes",
		Help:    "Peak resident bytes of one Rust image worker process where the host reports it.",
		Buckets: prometheus.ExponentialBuckets(16<<20, 2, 7),
	}, []string{"kind"})
	imageWorkerQueueWait = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "loomarr", Subsystem: "image_worker", Name: "queue_wait_seconds",
		Help:    "Time spent waiting for image worker capacity by admission class.",
		Buckets: []float64{0.0001, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 1, 5, 30},
	}, []string{"class"})
	imageWorkerInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "loomarr", Subsystem: "image_worker", Name: "in_flight",
		Help: "Rust image worker processes currently holding a global image slot.",
	})
)

// ImageWorkerObserved records one bounded process-boundary observation from rustgen.
func ImageWorkerObserved(observation rustgen.Observation) {
	imageWorkerOperations.WithLabelValues(observation.Kind, observation.Result).Inc()
	imageWorkerDuration.WithLabelValues(observation.Kind).Observe(observation.Duration.Seconds())
	imageWorkerInputBytes.WithLabelValues(observation.Kind).Observe(float64(observation.InputBytes))
	imageWorkerOutputBytes.WithLabelValues(observation.Kind).Observe(float64(observation.OutputBytes))
	if observation.PeakRSSBytes > 0 {
		imageWorkerPeakRSSBytes.WithLabelValues(observation.Kind).Observe(float64(observation.PeakRSSBytes))
	}
}

// ImageWorkerQueueWait records one attempt to acquire image capacity by its stable admission class.
func ImageWorkerQueueWait(class string, wait time.Duration) {
	imageWorkerQueueWait.WithLabelValues(class).Observe(wait.Seconds())
}

// ImageWorkerInFlight applies the service's balanced +1/-1 process transition.
func ImageWorkerInFlight(delta int) { imageWorkerInFlight.Add(float64(delta)) }
