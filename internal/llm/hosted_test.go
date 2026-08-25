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

// The hosted surface is exactly two entries (§8.1): OpenRouter (curated base) and
// Custom (empty base — the user supplies it). The former per-vendor entries
// (openai/anthropic/groq/gemini) are NOT curated providers anymore — they're
// reached via OpenRouter or a Custom base, so they must not resolve as providers.
func TestHostedCatalog_OpenRouterAndCustomOnly(t *testing.T) {
	cat := HostedCatalog()
	if len(cat) != 2 {
		t.Fatalf("want exactly 2 hosted entries (openrouter, custom), got %d: %+v", len(cat), cat)
	}
	cp, ok := HostedProviderByKey(CustomProviderKey)
	if !ok {
		t.Fatal("custom must be in the catalog as a template")
	}
	if cp.BaseURL != "" {
		t.Errorf("custom template must carry no base URL (the user supplies it), got %q", cp.BaseURL)
	}
	for _, dropped := range []string{"openai", "anthropic", "groq", "gemini"} {
		if _, ok := HostedProviderByKey(dropped); ok {
			t.Errorf("%q must NOT be a curated provider anymore (reach it via OpenRouter or Custom)", dropped)
		}
	}
}

func TestHostedCatalog_OpenRouterFallbackIsNotAnUncertifiedRecommendation(t *testing.T) {
	hp, ok := HostedProviderByKey("openrouter")
	if !ok {
		t.Fatal("openrouter should be in the curated catalog")
	}
	if len(hp.Fallback) != 1 {
		t.Fatalf("openrouter fallback = %+v, want one deliberate safe default", hp.Fallback)
	}
	model := hp.Fallback[0]
	if model.Recommended || !model.Tools || model.Why != "" {
		t.Errorf("fallback = %+v, want a capable placeholder without a quality claim", model)
	}
}

// RICH provider (OpenRouter-shape): LiveModels projects capabilities and preserves
// provider order. Quality recommendations belong only to a certified role policy.
func TestLiveModels_PreservesProviderOrderWithoutQualityClaims(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			w.WriteHeader(404)
			return
		}
		if r.URL.Query().Get("output_modalities") != "all" {
			t.Errorf("output_modalities = %q, want all so every role is discoverable", r.URL.Query().Get("output_modalities"))
		}
		// The provider order is deliberate. Loomarr may filter incompatible entries,
		// but must not reshuffle these into a stale family hierarchy.
		_, _ = w.Write([]byte(`{"data":[
			{"id":"openai/gpt-4.1-nano:batch","name":"GPT-4.1 nano Batch","context_length":128000,"supported_parameters":["tools"],"pricing":{"prompt":"0.0000001","completion":"0.0000004"}},
			{"id":"someorg/free-coder","name":"FreeCoder","context_length":1000000,"supported_parameters":["tools"],"pricing":{"prompt":"0","completion":"0"}},
			{"id":"openai/gpt-4o","name":"GPT-4o","context_length":128000,"supported_parameters":["tools"],"pricing":{"prompt":"0.0000025","completion":"0.00001"}},
			{"id":"anthropic/claude-haiku-4.5","name":"Claude Haiku","context_length":200000,"supported_parameters":["tools"],"pricing":{"prompt":"0.0000008","completion":"0.000004"}},
			{"id":"google/gemini-vision","name":"Gemini Vision","architecture":{"input_modalities":["text","image"],"output_modalities":["text"]}},
			{"id":"openai/whisper-large-v3","name":"Whisper","architecture":{"input_modalities":["audio"],"output_modalities":["transcription"]}},
			{"id":"no/tools","name":"NoTools","supported_parameters":["temperature"],"pricing":{"prompt":"0","completion":"0"}}
		]}`))
	}))
	defer srv.Close()

	hp := HostedProvider{Key: "openrouter", BaseURL: srv.URL}
	models, live := hp.LiveModels(context.Background(), "key")
	if !live {
		t.Fatal("expected live=true")
	}
	// Three lineup choices plus one authoritative choice for each other role.
	if len(models) != 5 {
		t.Fatalf("got %d role-capable models, want 5", len(models))
	}
	if models[0].ID != "someorg/free-coder" || models[1].ID != "openai/gpt-4o" || models[2].ID != "anthropic/claude-haiku-4.5" {
		t.Errorf("provider order changed: %+v", models)
	}
	for _, model := range models {
		if strings.HasSuffix(model.ID, ":batch") {
			t.Errorf("batch-only model remained selectable: %+v", model)
		}
		if model.Recommended || model.Why != "" {
			t.Errorf("generic catalog made an uncertified quality claim: %+v", model)
		}
	}
	if !models[3].Vision || models[3].Tools {
		t.Errorf("vision model capabilities = %+v, want vision-only", models[3])
	}
	if !models[4].Transcription || models[4].Tools {
		t.Errorf("transcription model capabilities = %+v, want transcription-only", models[4])
	}
}

func TestLiveModels_DoesNotDemoteGPT5OrGemini3BehindLegacyFamilies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"openai/gpt-5-mini","supported_parameters":["tools"]},
			{"id":"google/gemini-3.7-flash","supported_parameters":["tools"]},
			{"id":"openai/gpt-4o","supported_parameters":["tools"]},
			{"id":"google/gemini-2.5-pro","supported_parameters":["tools"]}
		]}`))
	}))
	defer srv.Close()

	hp := HostedProvider{Key: "openrouter", BaseURL: srv.URL}
	models, live := hp.LiveModels(context.Background(), "key")
	if !live || len(models) != 4 {
		t.Fatalf("models = %+v, live=%v; want four live candidates", models, live)
	}
	if models[0].ID != "openai/gpt-5-mini" || models[1].ID != "google/gemini-3.7-flash" {
		t.Errorf("new families were demoted behind legacy tiers: %+v", models)
	}
	for _, model := range models {
		if model.Recommended {
			t.Errorf("live capability metadata cannot certify %q", model.ID)
		}
	}
}

// THIN provider (OpenAI/Groq-shape: just ids, no metadata): rules can't rank, so it
// degrades gracefully to the live id list — current, unranked, no fabricated stars.
func TestLiveModels_ThinMetadataDegrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4o"}]}`))
	}))
	defer srv.Close()

	hp := HostedProvider{Key: "openrouter", BaseURL: srv.URL}
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

func TestLiveModels_PreservesAdvertisedVideoOnlyModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"id":"google/video-model","architecture":{"input_modalities":["text","video"],"output_modalities":["text"]}}
		]}`))
	}))
	defer srv.Close()

	models, live := (HostedProvider{Key: "openrouter", BaseURL: srv.URL}).LiveModels(context.Background(), "key")
	if !live || len(models) != 1 {
		t.Fatalf("models = %+v, live=%v; want advertised video model preserved", models, live)
	}
	if !models[0].Video || models[0].Vision || models[0].Tools {
		t.Fatalf("capabilities = %+v, want video-only", models[0])
	}
}

func TestLiveModels_ThinMetadataExcludesBatchOnlyVariants(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"openai/gpt-4.1-nano:batch"},{"id":"openai/gpt-4.1-nano"}]}`))
	}))
	defer srv.Close()

	hp := HostedProvider{Key: "openrouter", BaseURL: srv.URL}
	models, live := hp.LiveModels(context.Background(), "key")
	if !live || len(models) != 1 || models[0].ID != "openai/gpt-4.1-nano" {
		t.Fatalf("thin catalog = %+v, live=%v; want only the synchronous model", models, live)
	}
}

func TestLiveModels_CustomPreservesProviderDefinedModelIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"local/model:batch"}]}`))
	}))
	defer srv.Close()

	hp := HostedProvider{Key: CustomProviderKey, BaseURL: srv.URL}
	models, live := hp.LiveModels(context.Background(), "key")
	if !live || len(models) != 1 || models[0].ID != "local/model:batch" {
		t.Fatalf("custom catalog = %+v, live=%v; custom ids must remain provider-defined", models, live)
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
