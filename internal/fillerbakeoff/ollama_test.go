package fillerbakeoff

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filleradmission"
)

func TestOllamaExtractorPinsDigestDisablesThinkingAndReturnsGroundedEvidence(t *testing.T) {
	digest := strings.Repeat("a", 64)
	var sent map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:26b-a4b-it-qat","model":"gemma4:26b-a4b-it-qat","digest":"` + digest + `"}]}`))
		case "/api/chat":
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "gemma4:26b-a4b-it-qat", "done_reason": "stop", "prompt_eval_count": 100, "eval_count": 20,
				"message": map[string]any{"role": "assistant", "content": `{"facts":[{"claim":"content_role","value":"promo","signal_id":"transcript","location":"spoken schedule"}],"abstention_reason":""}`},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	times := []time.Time{time.Unix(0, 0), time.Unix(0, int64(125*time.Millisecond))}
	extractor, err := NewOllamaExtractor(context.Background(), OllamaConfig{
		BaseURL: server.URL, Model: "gemma4:26b-a4b-it-qat", ModelDigest: digest,
		Now: func() time.Time { value := times[0]; times = times[1:]; return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	route := Route{Class: RouteText, Role: "filler_text", Rung: "text", Provider: "ollama", Model: "gemma4:26b-a4b-it-qat", Modalities: []string{"text"}, StructuredOutput: true, MaxChargeNanoUSD: 1, MaxAttempts: 1, EscalateOn: []filleradmission.ReasonCode{filleradmission.ReasonMissingContentRole}}
	result, err := extractor.Extract(context.Background(), Request{Packet: Packet{Signals: []Signal{{ID: "transcript", Kind: string(filleradmission.KindTranscript), Text: "Tonight at eight."}}}, Route: route, Reasons: route.EscalateOn})
	if err != nil {
		t.Fatal(err)
	}
	if sent["think"] != false || sent["stream"] != false || sent["format"] == "json" {
		t.Fatalf("request = %+v", sent)
	}
	if got := sent["options"].(map[string]any)["num_predict"]; got != float64(maxOpenRouterOutputTokens) {
		t.Fatalf("num_predict = %v", got)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Value != "promo" || result.Evidence[0].EvaluationID != result.Attribution.EvaluationID {
		t.Fatalf("extraction = %+v", result)
	}
	if result.Attribution.ResolvedModel != "gemma4:26b-a4b-it-qat@sha256:"+digest || result.Attribution.LatencyMS != 125 || result.Attribution.Tokens.Prompt != 100 || result.Attribution.Tokens.Completion != 20 {
		t.Fatalf("attribution = %+v", result.Attribution)
	}
}

func TestOllamaExtractorFailsClosedOnEndpointAndDigestDrift(t *testing.T) {
	if _, err := NewOllamaExtractor(context.Background(), OllamaConfig{BaseURL: "https://example.com", Model: "gemma4:qat", ModelDigest: strings.Repeat("a", 64)}); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("endpoint error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:qat","digest":"` + strings.Repeat("b", 64) + `"}]}`))
	}))
	defer server.Close()
	if _, err := NewOllamaExtractor(context.Background(), OllamaConfig{BaseURL: server.URL, Model: "gemma4:qat", ModelDigest: strings.Repeat("a", 64)}); err == nil || !strings.Contains(err.Error(), "not installed at digest") {
		t.Fatalf("digest error = %v", err)
	}
}
