package suggest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
)

func TestToolDiagnosticReportsEffectiveSampling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sent map[string]any
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
			return
		}
		if sent["temperature"] != nil || sent["tools"] != nil || sent["response_format"] == nil {
			t.Error("diagnostic wire differs from finalization profile")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer server.Close()
	const model = "google/gemini-3.8-flash"
	provider := llm.NewOpenAIForProvider("openrouter", server.URL, model, "")
	report, err := suggest.RunToolFinalizationDiagnostic(context.Background(), provider, model)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 2 || report.ParameterProfile != llm.GroundedSelection || report.Temperature != nil || report.ThinkingSetting != "reasoning-effort-low-v1" {
		t.Fatalf("effective sampling not reported: %+v", report)
	}
}
