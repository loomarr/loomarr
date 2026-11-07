package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/metrics"
)

// Every Target a caller can name must surface under its own bounded metric
// label. A Target the recorder does not know collapses into target="other",
// which hides which remote service is slow or failing.
func TestEveryTargetHasItsOwnMetricLabel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	for _, target := range Targets() {
		t.Run(string(target), func(t *testing.T) {
			label := target.Label()
			if label == "" || label == "other" {
				t.Fatalf("Target %q has label %q, want a registered service label", target, label)
			}
			recorder := metrics.New(metrics.Options{})
			resp, err := NewNamedObserved(target, TimeoutProbe, recorder).Get(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()

			want := `loomarr_outbound_requests_total{code="204",target="` + label + `"} 1`
			if body := scrapeMetrics(t, recorder); !strings.Contains(body, want) {
				t.Errorf("scrape does not contain %q:\n%s", want, body)
			}
		})
	}
}

func TestUnknownTargetLabelsAsOther(t *testing.T) {
	if got := Target("not-a-service").Label(); got != "other" {
		t.Fatalf("Label() = %q, want other", got)
	}
}

func TestTargetsAreUniqueAndListed(t *testing.T) {
	seen := map[Target]bool{}
	for _, target := range Targets() {
		if seen[target] {
			t.Errorf("Target %q listed twice", target)
		}
		seen[target] = true
	}
	for _, want := range []Target{
		TargetReference, TargetFillerWebSearch, TargetFillerStructuredResearch,
		TargetFillerStructureWindow, TargetFillerOpenRouterSnapshot,
	} {
		if !seen[want] {
			t.Errorf("Target %q missing from Targets()", want)
		}
	}
}

func scrapeMetrics(t *testing.T, recorder *metrics.Recorder) string {
	t.Helper()
	rec := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
