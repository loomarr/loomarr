package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeDistinguishesUnsupportedFromUnverifiedToolCapability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"1.0"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"known-no-tools"},{"name":"show-unavailable"}]}`))
		case "/api/show":
			if r.Body != nil {
				defer func() { _ = r.Body.Close() }()
			}
			// The request body is tiny; distinguishing by a simple buffer keeps the test
			// about the provider response states rather than JSON parsing machinery.
			buf := make([]byte, 256)
			n, _ := r.Body.Read(buf)
			if string(buf[:n]) == `{"model":"known-no-tools"}` {
				_, _ = w.Write([]byte(`{"capabilities":["completion"]}`))
				return
			}
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	probe := NewProber(srv.URL).Probe(context.Background())
	if len(probe.Installed) != 2 {
		t.Fatalf("installed = %+v, want two models", probe.Installed)
	}
	if got := probe.Installed[0].ToolCapability; got != ToolCapabilityUnsupported {
		t.Errorf("declared no-tools capability = %q, want unsupported", got)
	}
	if got := probe.Installed[1].ToolCapability; got != ToolCapabilityUnverified {
		t.Errorf("failed capability probe = %q, want unverified", got)
	}
}
