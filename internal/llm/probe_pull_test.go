package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Pull must hand the callback the RAW byte counts from each Ollama stream line —
// not just a collapsed percent — so a UI can render "X of Y GB" and derive ETA
// from successive frames (§8.1). This is the regression guard for that contract.
func TestPull_SurfacesCompletedAndTotalBytes(t *testing.T) {
	// Two progress lines then a final success — the shape Ollama streams.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fl, _ := w.(http.Flusher)
		lines := []string{
			`{"status":"pulling manifest"}`,
			`{"status":"pulling abc","completed":250,"total":1000}`,
			`{"status":"pulling abc","completed":1000,"total":1000}`,
			`{"status":"success"}`,
		}
		for _, ln := range lines {
			if _, err := fmt.Fprintln(w, ln); err != nil {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	var got []PullProgress
	err := NewProber(srv.URL).Pull(context.Background(), "qwen3.5:9b", func(pp PullProgress) {
		got = append(got, pp)
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 progress frames, got %d: %+v", len(got), got)
	}
	// Manifest line: no totals yet -> Percent unknown, bytes zero.
	if got[0].Total != 0 || got[0].Percent() != -1 {
		t.Errorf("manifest frame: want total=0/percent=-1, got total=%d/percent=%d", got[0].Total, got[0].Percent())
	}
	// Mid-download: raw bytes preserved, percent derived.
	if got[1].Completed != 250 || got[1].Total != 1000 || got[1].Percent() != 25 {
		t.Errorf("mid frame: want 250/1000/25%%, got %d/%d/%d%%", got[1].Completed, got[1].Total, got[1].Percent())
	}
	if got[2].Percent() != 100 {
		t.Errorf("complete frame: want 100%%, got %d%%", got[2].Percent())
	}
}

// A stream line carrying an "error" field fails the pull.
func TestPull_ErrorLineFailsPull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `{"status":"pulling","error":"model not found"}`)
	}))
	defer srv.Close()

	err := NewProber(srv.URL).Pull(context.Background(), "bogus", func(PullProgress) {})
	if err == nil {
		t.Fatal("expected an error from a stream error line, got nil")
	}
}
