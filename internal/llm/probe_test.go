package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeReadsAllInstalledModelCapabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"1.2.3"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5vl:7b","size":5368709120,"details":{"family":"qwen","parameter_size":"7B","quantization_level":"Q4_K_M"}}]}`))
		case "/api/show":
			_, _ = w.Write([]byte(`{"capabilities":["completion","tools","vision"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := NewProber(srv.URL)
	p.nvidiaSMI = func(context.Context) (string, float64, bool) { return "", 0, false }
	probe := p.Probe(context.Background())
	if len(probe.Installed) != 1 {
		t.Fatalf("installed = %+v, want one model", probe.Installed)
	}
	if !probe.Installed[0].Tools || !probe.Installed[0].Vision {
		t.Fatalf("capabilities = %+v, want tools and vision", probe.Installed[0])
	}
}
