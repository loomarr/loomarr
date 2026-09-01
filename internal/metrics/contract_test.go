package metrics

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/images/rustgen"
)

func TestRecorderMatchesReviewedMetricManifest(t *testing.T) {
	recorder := New(Options{
		Version: "test", Revision: "revision", Database: "sqlite",
		Store: fakeCounts{}, Now: time.Now, DatabaseStats: func() sql.DBStats { return sql.DBStats{} },
	})
	recorder.LoginResult(true)
	recorder.LLMTokens(1, 1)
	recorder.FillerPodAssembled("exact")
	recorder.FillerRotationAired(false, false, false)
	recorder.ChannelReconciled(time.Millisecond, true)
	recorder.ChannelSlotSubstitutions(1)
	recorder.SchedulerJobs([]string{"test-job"})
	recorder.SchedulerJobStarted("test-job")
	recorder.SchedulerJobFinished("test-job", "success", "scheduled", time.Millisecond, time.Now())
	recorder.PlayoutSessionStarted("success")
	recorder.PlayoutSessionActive(1)
	recorder.PlayoutProcessFailure("parent")
	recorder.PlayoutFallback("prepared_to_live")
	recorder.ImageWorkerObserved(rustgen.Observation{
		Kind: "inspect", Result: "success", Duration: time.Millisecond, PeakRSSBytes: 1,
	})
	recorder.ImageWorkerQueueWait("interactive", time.Millisecond)
	recorder.ImageWorkerInFlight(1)

	transport := recorder.InstrumentTransport("library", contractRoundTripper{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /contract/{id}", func(w http.ResponseWriter, request *http.Request) {
		response, err := transport.RoundTrip(request.Clone(request.Context()))
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		w.WriteHeader(http.StatusNoContent)
	})
	recorder.FanoutMiddleware(recorder.Middleware(mux)).ServeHTTP(
		httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/contract/private", nil),
	)

	families, err := recorder.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	actual := make([]string, 0, len(families))
	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), "loomarr_") {
			continue
		}
		labels := map[string]struct{}{}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() != "le" && label.GetName() != "quantile" {
					labels[label.GetName()] = struct{}{}
				}
			}
		}
		labelNames := make([]string, 0, len(labels))
		for label := range labels {
			labelNames = append(labelNames, label)
		}
		slices.Sort(labelNames)
		actual = append(actual, family.GetName()+"|"+strings.ToLower(family.GetType().String())+"|"+strings.Join(labelNames, ","))
	}
	slices.Sort(actual)

	raw, err := os.ReadFile("../../observability/metrics-manifest.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := make([]string, 0, len(actual))
	for line := range strings.Lines(string(raw)) {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			want = append(want, line)
		}
	}
	slices.Sort(want)
	if strings.Join(actual, "\n") != strings.Join(want, "\n") {
		t.Fatalf("metric manifest drift\nactual:\n%s\n\nwant:\n%s", strings.Join(actual, "\n"), strings.Join(want, "\n"))
	}
}

type contractRoundTripper struct{}

func (contractRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}
