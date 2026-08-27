package fillerbakeoff

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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
	for _, message := range sent["messages"].([]any) {
		if _, exists := message.(map[string]any)["images"]; exists {
			t.Fatalf("text request acquired an images field: %+v", sent)
		}
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Value != "promo" || result.Evidence[0].EvaluationID != result.Attribution.EvaluationID {
		t.Fatalf("extraction = %+v", result)
	}
	if result.Attribution.ResolvedModel != "gemma4:26b-a4b-it-qat@sha256:"+digest || result.Attribution.LatencyMS != 125 || result.Attribution.Tokens.Prompt != 100 || result.Attribution.Tokens.Completion != 20 {
		t.Fatalf("attribution = %+v", result.Attribution)
	}
}

func TestOllamaExtractorSendsVerifiedFramesInPacketOrder(t *testing.T) {
	digest := strings.Repeat("c", 64)
	var sent ollamaBakeoffRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3-vl:8b-instruct","digest":"` + digest + `"}]}`))
		case "/api/chat":
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "qwen3-vl:8b-instruct", "done_reason": "stop", "prompt_eval_count": 80, "eval_count": 12,
				"message": map[string]any{"role": "assistant", "content": `{"facts":[{"claim":"content_role","value":"commercial","signal_id":"frame-2","location":"product end card"}],"abstention_reason":""}`},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	extractor, err := NewOllamaExtractor(context.Background(), OllamaConfig{BaseURL: server.URL, Model: "qwen3-vl:8b-instruct", ModelDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	route := Route{Class: RouteFrames, Role: "filler_frames", Rung: "frames", Provider: "ollama", Model: "qwen3-vl:8b-instruct", Modalities: []string{"image"}, StructuredOutput: true, MaxChargeNanoUSD: 1, MaxAttempts: 1, EscalateOn: []filleradmission.ReasonCode{filleradmission.ReasonMissingContentRole}}
	first, second := []byte("first verified jpeg"), []byte("second verified jpeg")
	packet := Packet{Signals: []Signal{
		{ID: "frame-2", Kind: string(filleradmission.KindFrame), Bytes: int64(len(first)), Width: 320, Height: 240, ContentTypes: []string{"image/jpeg"}},
		{ID: "frame-1", Kind: string(filleradmission.KindFrame), Bytes: int64(len(second)), Width: 320, Height: 240, ContentTypes: []string{"image/jpeg"}},
	}}
	result, err := extractor.Extract(context.Background(), Request{Packet: packet, Route: route, SignalData: map[string][]byte{"frame-2": first, "frame-1": second}, Reasons: route.EscalateOn})
	if err != nil {
		t.Fatal(err)
	}
	if len(sent.Messages) != 2 || len(sent.Messages[0].Images) != 0 || !slices.Equal(sent.Messages[1].Images, []string{base64.StdEncoding.EncodeToString(first), base64.StdEncoding.EncodeToString(second)}) {
		t.Fatalf("messages = %+v", sent.Messages)
	}
	if !strings.Contains(sent.Messages[1].Content, "frame-2") || !strings.Contains(sent.Messages[1].Content, "frame-1") {
		t.Fatalf("frame identities absent from prompt: %q", sent.Messages[1].Content)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Derivative != "frame-2" || !slices.Equal(result.Attribution.Modalities, []string{"image"}) {
		t.Fatalf("extraction = %+v", result)
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
