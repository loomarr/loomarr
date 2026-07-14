package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostedProviderByKey(t *testing.T) {
	if _, ok := HostedProviderByKey("openrouter"); !ok {
		t.Error("openrouter should be in the curated catalog")
	}
	if _, ok := HostedProviderByKey("nope"); ok {
		t.Error("unknown provider must not resolve")
	}
}

// RICH provider (OpenRouter-shape): LiveModels filters to tool-capable models and
// ranks them cheapest-first — recommendations are DERIVED from live metadata, not
// hardcoded ids, so they can't rot.
func TestLiveModels_RanksRichMetadataByRules(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			w.WriteHeader(404)
			return
		}
		// three models: a cheap tool-caller, an expensive tool-caller, and a
		// NON-tool model that must be filtered out of recommendations.
		_, _ = w.Write([]byte(`{"data":[
			{"id":"pricey/tools","name":"Pricey","context_length":200000,"supported_parameters":["tools"],"pricing":{"prompt":"0.00001","completion":"0.00003"}},
			{"id":"cheap/tools","name":"Cheap","context_length":128000,"supported_parameters":["tools"],"pricing":{"prompt":"0.00000015","completion":"0.0000006"}},
			{"id":"no/tools","name":"NoTools","context_length":8000,"supported_parameters":["temperature"],"pricing":{"prompt":"0.0000001","completion":"0.0000001"}}
		]}`))
	}))
	defer srv.Close()

	hp := HostedProvider{BaseURL: srv.URL}
	models, live := hp.LiveModels(context.Background(), "key")
	if !live {
		t.Fatal("expected live=true")
	}
	// The non-tool model is dropped by the hard filter (grounding needs tools).
	for _, m := range models {
		if m.ID == "no/tools" {
			t.Error("a non-tool-capable model must not survive the tool filter")
		}
	}
	if len(models) != 2 {
		t.Fatalf("got %d tool-capable models, want 2", len(models))
	}
	// Cheapest tool-caller ranks first and is recommended with a rule-derived Why.
	if models[0].ID != "cheap/tools" || !models[0].Recommended {
		t.Errorf("first = %+v, want cheap/tools recommended", models[0])
	}
	if models[0].Why == "" || !strings.Contains(models[0].Why, "tool-calling") {
		t.Errorf("Why should be rule-derived, got %q", models[0].Why)
	}
	if !models[0].Tools {
		t.Error("ranked model should carry the tools flag")
	}
}

// THIN provider (OpenAI/Groq-shape: just ids, no metadata): rules can't rank, so it
// degrades gracefully to the live id list — current, unranked, no fabricated stars.
func TestLiveModels_ThinMetadataDegrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4o"}]}`))
	}))
	defer srv.Close()

	hp := HostedProvider{BaseURL: srv.URL}
	models, live := hp.LiveModels(context.Background(), "key")
	if !live || len(models) != 2 {
		t.Fatalf("thin provider should still return the 2 live ids, got live=%v n=%d", live, len(models))
	}
	for _, m := range models {
		if m.Recommended {
			t.Error("thin metadata must NOT produce recommendations (no data to rank on)")
		}
	}
}

// On an unreachable provider (or no key), LiveModels falls back to the tiny curated
// Fallback with live=false — a placeholder beats an empty picker.
func TestLiveModels_FallbackWhenUnreachable(t *testing.T) {
	hp := HostedProvider{
		BaseURL:  "http://127.0.0.1:0", // unreachable
		Fallback: []HostedModel{{ID: "a", Label: "A"}},
	}
	models, live := hp.LiveModels(context.Background(), "key")
	if live {
		t.Error("unreachable provider should report live=false")
	}
	if len(models) != 1 || models[0].ID != "a" {
		t.Errorf("should return the curated fallback, got %+v", models)
	}
}

// ValidateKey exercises the KEY via chat/completions (NOT /models, which is public
// on some providers). 401 → error; 2xx → ok; 400 (key accepted, bad request) → ok.
func TestValidateKey(t *testing.T) {
	// Guard against the OpenRouter gotcha: a handler that 200s on /models but 401s on
	// /chat/completions must be treated as a BAD key (we must hit completions).
	unauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"x"}]}`)) // public catalog, 200
			return
		}
		w.WriteHeader(http.StatusUnauthorized) // completions rejects the bad key
	}))
	defer unauth.Close()
	if err := ValidateKey(context.Background(), unauth.URL, "bad"); err == nil {
		t.Error("expected error when chat/completions 401s (even though /models 200s)")
	}

	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
	}))
	defer ok.Close()
	if err := ValidateKey(context.Background(), ok.URL, "good"); err != nil {
		t.Errorf("expected success on 2xx, got %v", err)
	}

	// 400 = key accepted, request/model rejected → still authorized.
	badReq := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"unknown model"}}`))
	}))
	defer badReq.Close()
	if err := ValidateKey(context.Background(), badReq.URL, "good"); err != nil {
		t.Errorf("400 (key accepted, bad model) should be treated as authorized, got %v", err)
	}
}
