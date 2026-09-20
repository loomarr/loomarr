package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerresearch"
	"github.com/loomarr/loomarr/internal/store"
)

func TestFillerResearchSettingsAdapterTestsSavedProviderWithoutReturningItsSecret(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Clip history","url":"https://example.org/clip","content":"Historical context."}]}`))
	}))
	defer server.Close()

	st, err := store.Open(t.Context(), "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	set := visionSet(t, map[string]string{
		"filler.research.enabled":       "true",
		"filler.research.web_provider":  "searxng",
		"filler.research.searxng_url":   server.URL,
		"filler.research.monthly_limit": "2",
	})
	adapter := newFillerResearchSettingsAdapter(st, set, nil)
	adapter.client = server.Client()
	adapter.now = func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) }

	status, ok, message, err := adapter.Test(t.Context(), fillerresearch.WebConfig{Provider: fillerresearch.WebProviderSearXNG})
	if err != nil || !ok || message != "Web search is ready." || requests != 1 {
		t.Fatalf("status=%+v ok=%v message=%q requests=%d err=%v", status, ok, message, requests, err)
	}
	if status.RequestCount != 1 || status.State != fillerresearch.WebStateReady || !status.Configured {
		t.Fatalf("status = %+v", status)
	}
}
