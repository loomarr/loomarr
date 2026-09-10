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
	if report.SchemaVersion != 3 || report.ParameterProfile != llm.GroundedSelection || report.Temperature != nil || report.ThinkingSetting != "reasoning-effort-low-v1" {
		t.Fatalf("effective sampling not reported: %+v", report)
	}
}

func TestToolDiagnosticReportsSolCompletionLimitAndDefaultRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sent map[string]any
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
			return
		}
		if sent["max_completion_tokens"] != float64(2048) || sent["max_tokens"] != nil || sent["tools"] != nil || sent["response_format"] == nil {
			t.Errorf("unexpected finalization parameters: %v", sent)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer server.Close()
	const model = "openai/gpt-5.6-sol"
	report, err := suggest.RunToolFinalizationDiagnostic(context.Background(), llm.NewOpenAIForProvider("openrouter", server.URL, model, ""), model)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 3 || report.CompletionLimitParameter != "max_completion_tokens" || report.DefaultUpstream != "azure/us" || report.MaxTokens != 2048 || report.ThinkingSetting != "reasoning-effort-none-v1" || report.Temperature != nil {
		t.Fatalf("effective request not reported: %+v", report)
	}
}
