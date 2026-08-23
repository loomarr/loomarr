package metrics

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// The instrumented transport records the response status on success and a
// distinct code="error" when the round trip itself fails (§17 Tunarr-API errors,
// generalised). Deltas because the counters live on the default registry.
func TestInstrumentTransportRecordsResult(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://svc/", nil)

	ok := InstrumentTransport("tunarr", httpfixture.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	}))
	before := testutil.ToFloat64(outboundRequests.WithLabelValues("tunarr", "200"))
	if _, err := ok.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(outboundRequests.WithLabelValues("tunarr", "200")); got != before+1 {
		t.Errorf("outbound{200} = %v, want %v", got, before+1)
	}

	failing := InstrumentTransport("tunarr", httpfixture.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}))
	eb := testutil.ToFloat64(outboundRequests.WithLabelValues("tunarr", "error"))
	if _, err := failing.RoundTrip(req); err == nil {
		t.Fatal("expected the transport error to propagate")
	}
	if got := testutil.ToFloat64(outboundRequests.WithLabelValues("tunarr", "error")); got != eb+1 {
		t.Errorf("outbound{error} = %v, want %v", got, eb+1)
	}
}

func TestOutboundRetriedRecordsBoundedReasons(t *testing.T) {
	tests := []struct {
		name   string
		reason OutboundRetryReason
		label  string
	}{
		{name: "transport", reason: OutboundRetryTransport, label: "transport"},
		{name: "request timeout", reason: OutboundRetryStatus408, label: "408"},
		{name: "rate limited", reason: OutboundRetryStatus429, label: "429"},
		{name: "internal error", reason: OutboundRetryStatus500, label: "500"},
		{name: "bad gateway", reason: OutboundRetryStatus502, label: "502"},
		{name: "unavailable", reason: OutboundRetryStatus503, label: "503"},
		{name: "gateway timeout", reason: OutboundRetryStatus504, label: "504"},
		{name: "unknown stays bounded", reason: OutboundRetryReason(255), label: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := testutil.ToFloat64(outboundRetries.WithLabelValues("tmdb", tt.label))
			OutboundRetried("tmdb", tt.reason)
			if got := testutil.ToFloat64(outboundRetries.WithLabelValues("tmdb", tt.label)); got != before+1 {
				t.Errorf("outbound retries{%s} = %v, want %v", tt.label, got, before+1)
			}
		})
	}
}

// ReconcileObserved routes success and failure to the right result label.
func TestReconcileObserved(t *testing.T) {
	sb := testutil.ToFloat64(channelReconciles.WithLabelValues("success"))
	fb := testutil.ToFloat64(channelReconciles.WithLabelValues("error"))

	ReconcileObserved(50*time.Millisecond, true)
	ReconcileObserved(0, false)

	if got := testutil.ToFloat64(channelReconciles.WithLabelValues("success")); got != sb+1 {
		t.Errorf("reconciles{success} = %v, want %v", got, sb+1)
	}
	if got := testutil.ToFloat64(channelReconciles.WithLabelValues("error")); got != fb+1 {
		t.Errorf("reconciles{error} = %v, want %v", got, fb+1)
	}
}
