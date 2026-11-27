package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// During verbose capture each request log is a stored diagnostic event, whose frame makes the
// Diagnostics page read again: so reading the log, or the stream, must not be logged, or the page
// would refresh itself for as long as it is open. Everything else still is.
func TestRequestLogLeavesOutReadsOfTheDiagnosticLog(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler := logRequests(log, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	for _, req := range []struct{ method, path string }{
		{http.MethodGet, "/v1/diagnostics/events"},
		{http.MethodGet, "/v1/diagnostics/verbose-capture"},
		{http.MethodGet, "/v1/events"},
		{http.MethodGet, "/v1/channels"},
		{http.MethodPost, "/v1/diagnostics/verbose-capture"},
	} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(req.method, req.path, nil))
	}

	logged := out.String()
	for _, quiet := range []string{"path=/v1/diagnostics/events", "path=/v1/events"} {
		if strings.Contains(logged, quiet) {
			t.Errorf("a read of the diagnostic log was logged: %s", quiet)
		}
	}
	if !strings.Contains(logged, "path=/v1/channels") ||
		!strings.Contains(logged, "method=POST path=/v1/diagnostics/verbose-capture") {
		t.Errorf("other requests must still be logged; got:\n%s", logged)
	}
}
