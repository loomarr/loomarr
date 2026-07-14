package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tierOf matches by FAMILY substring so provider prefixes + dated snapshots hit the
// same tier — the durability property that keeps this from rotting on exact ids.
func TestTierOf_MatchesFamiliesAcrossPrefixesAndDates(t *testing.T) {
	cases := []struct {
		id       string
		wantTier int
	}{
		{"openai/gpt-4o", 3},              // OpenRouter prefix
		{"gpt-4o-mini", 3},                // OpenAI direct
		{"gpt-4o-mini-2026-05-01", 3},     // dated snapshot still matches
		{"anthropic/claude-haiku-4.5", 2}, // tier-2 workhorse
		{"meta-llama/llama-3.3-70b", 2},
		{"someorg/unknown-model", 0}, // untiered → tier 0
	}
	for _, c := range cases {
		if got, _ := tierOf(c.id); got != c.wantTier {
			t.Errorf("tierOf(%q) = %d, want %d", c.id, got, c.wantTier)
		}
	}
}

func TestHostedProviderByKey(t *testing.T) {
	if _, ok := HostedProviderByKey("openrouter"); !ok {
		t.Error("openrouter should be in the curated catalog")
	}
	if _, ok := HostedProviderByKey("nope"); ok {
		t.Error("unknown provider must not resolve")
	}
}

// RICH provider (OpenRouter-shape): LiveModels ranks for the USE CASE — a curated
// quality FAMILY tier beats a cheaper-but-lower-tier model, and an untiered model
// (even if cheapest + tool-capable) is shown but NOT recommended. This is the
// "best for grounding, not merely cheapest" behavior.
func TestLiveModels_RanksByQualityTierThenCost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			w.WriteHeader(404)
			return
		}
		// - a FREE untiered coding model (cheapest, tool-capable) — must NOT be recommended
		// - a tier-3 gpt-4o family model (pricier) — SHOULD be recommended first
		// - a tier-2 claude-haiku family model (cheaper than gpt-4o) — recommended after
		// - a non-tool model — filtered out
		_, _ = w.Write([]byte(`{"data":[
			{"id":"someorg/free-coder","name":"FreeCoder","context_length":1000000,"supported_parameters":["tools"],"pricing":{"prompt":"0","completion":"0"}},
			{"id":"openai/gpt-4o","name":"GPT-4o","context_length":128000,"supported_parameters":["tools"],"pricing":{"prompt":"0.0000025","completion":"0.00001"}},
			{"id":"anthropic/claude-haiku-4.5","name":"Claude Haiku","context_length":200000,"supported_parameters":["tools"],"pricing":{"prompt":"0.0000008","completion":"0.000004"}},
			{"id":"no/tools","name":"NoTools","supported_parameters":["temperature"],"pricing":{"prompt":"0","completion":"0"}}
		]}`))
	}))
	defer srv.Close()

	hp := HostedProvider{BaseURL: srv.URL}
	models, live := hp.LiveModels(context.Background(), "key")
	if !live {
		t.Fatal("expected live=true")
	}
	// non-tool filtered out; the other three remain.
	if len(models) != 3 {
		t.Fatalf("got %d tool-capable models, want 3", len(models))
	}
	// Quality tier wins: gpt-4o (tier 3) ranks first despite being pricier than the
	// free coder and pricier than haiku.
	if models[0].ID != "openai/gpt-4o" || !models[0].Recommended {
		t.Errorf("first = %+v, want openai/gpt-4o recommended (tier beats cheaper)", models[0])
	}
	// The Why names the family (the quality signal), not just cost.
	if !strings.Contains(models[0].Why, "GPT-4o") {
		t.Errorf("Why should name the curated family, got %q", models[0].Why)
	}
	// The FREE untiered coder is present but must NOT be recommended.
	var coder *HostedModel
	for i := range models {
		if models[i].ID == "someorg/free-coder" {
			coder = &models[i]
		}
	}
	if coder == nil {
		t.Fatal("the untiered free coder should still be selectable")
	}
	if coder.Recommended {
		t.Error("an untiered model must never be recommended, even if cheapest")
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
