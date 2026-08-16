package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/mantonx/loomarr/internal/images/rustgen"
)

func TestImageWorkerMetricsObserveProcessAndQueueBoundaries(t *testing.T) {
	operationsBefore := testutil.ToFloat64(imageWorkerOperations.WithLabelValues("render", "success"))
	durationBefore := histogramCount(t, imageWorkerDuration.WithLabelValues("render"))
	inputBefore := histogramCount(t, imageWorkerInputBytes.WithLabelValues("render"))
	outputBefore := histogramCount(t, imageWorkerOutputBytes.WithLabelValues("render"))
	rssBefore := histogramCount(t, imageWorkerPeakRSSBytes.WithLabelValues("render"))
	queueBefore := histogramCount(t, imageWorkerQueueWait.WithLabelValues("background"))
	inFlightBefore := testutil.ToFloat64(imageWorkerInFlight)

	ImageWorkerObserved(rustgen.Observation{
		Kind: "render", Result: "success", Duration: 250 * time.Millisecond,
		InputBytes: 1024, OutputBytes: 512, PeakRSSBytes: 32 << 20,
	})
	ImageWorkerQueueWait("background", 5*time.Millisecond)
	ImageWorkerInFlight(1)

	if got := testutil.ToFloat64(imageWorkerOperations.WithLabelValues("render", "success")); got != operationsBefore+1 {
		t.Errorf("operations = %v, want %v", got, operationsBefore+1)
	}
	for name, got := range map[string]uint64{
		"duration": histogramCount(t, imageWorkerDuration.WithLabelValues("render")) - durationBefore,
		"input":    histogramCount(t, imageWorkerInputBytes.WithLabelValues("render")) - inputBefore,
		"output":   histogramCount(t, imageWorkerOutputBytes.WithLabelValues("render")) - outputBefore,
		"rss":      histogramCount(t, imageWorkerPeakRSSBytes.WithLabelValues("render")) - rssBefore,
		"queue":    histogramCount(t, imageWorkerQueueWait.WithLabelValues("background")) - queueBefore,
	} {
		if got != 1 {
			t.Errorf("%s histogram added %d samples, want 1", name, got)
		}
	}
	if got := testutil.ToFloat64(imageWorkerInFlight); got != inFlightBefore+1 {
		t.Errorf("in-flight = %v, want %v", got, inFlightBefore+1)
	}
	ImageWorkerInFlight(-1)
	if got := testutil.ToFloat64(imageWorkerInFlight); got != inFlightBefore {
		t.Errorf("balanced in-flight = %v, want %v", got, inFlightBefore)
	}
}

func histogramCount(t *testing.T, observer prometheus.Observer) uint64 {
	t.Helper()
	metric, ok := observer.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer %T is not a metric", observer)
	}
	var out dto.Metric
	if err := metric.Write(&out); err != nil {
		t.Fatal(err)
	}
	return out.GetHistogram().GetSampleCount()
}
