package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// LiveModels overlays curated annotations onto the provider's live /models list:
// recommended-and-still-live ids come first (annotated), then the rest.
func TestLiveModels_OverlaysCuratedOntoLive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			w.WriteHeader(404)
			return
		}
		// A live list: one curated id (gpt-4o-mini) + two the catalog doesn't annotate.
		_, _ = w.Write([]byte(`{"data":[{"id":"some/other-model"},{"id":"openai/gpt-4o-mini"},{"id":"third/model"}]}`))
	}))
	defer srv.Close()

	hp := HostedProvider{
		BaseURL: srv.URL,
		Models:  []HostedModel{{ID: "openai/gpt-4o-mini", Label: "GPT-4o mini", Why: "cheap"}},
	}
	models, live := hp.LiveModels(context.Background(), "key")
	if !live {
		t.Fatal("expected live=true when /models returns")
	}
	if len(models) != 3 {
		t.Fatalf("got %d models, want 3 (all live ids present)", len(models))
	}
	// The curated recommendation is surfaced first, with its annotation intact.
	if models[0].ID != "openai/gpt-4o-mini" || models[0].Why != "cheap" {
		t.Errorf("first model = %+v, want the annotated curated recommendation first", models[0])
	}
	// A non-curated live id is still selectable (bare entry).
	var foundBare bool
	for _, m := range models {
		if m.ID == "third/model" {
			foundBare = true
		}
	}
	if !foundBare {
		t.Error("a live but non-curated id should still appear (selectable)")
	}
}

// On an unreachable provider (or no key), LiveModels falls back to the curated list
// with live=false — a stale-but-present picker beats an empty one.
func TestLiveModels_FallbackWhenUnreachable(t *testing.T) {
	hp := HostedProvider{
		BaseURL: "http://127.0.0.1:0", // unreachable
		Models:  []HostedModel{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
	}
	models, live := hp.LiveModels(context.Background(), "key")
	if live {
		t.Error("unreachable provider should report live=false")
	}
	if len(models) != 2 {
		t.Errorf("fallback should return the 2 curated models, got %d", len(models))
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
