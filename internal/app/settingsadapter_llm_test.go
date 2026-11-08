package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// stubHostedModels replaces the live model listing for one test, so no test dials a provider.
func stubHostedModels(t *testing.T, list func(ctx context.Context, baseURL, apiKey string) error) {
	t.Helper()
	previous := checkHostedModels
	checkHostedModels = list
	t.Cleanup(func() { checkHostedModels = previous })
}

func TestLLMConnectionCheck_AcceptsSelectedProvidersNamespacedKey(t *testing.T) {
	var gotKey string
	stubHostedModels(t, func(_ context.Context, _, apiKey string) error { gotKey = apiKey; return nil })
	set := visionSet(t, map[string]string{
		"llm.provider":           "openai",
		"llm.hosted_provider":    "openrouter",
		"llm.url":                "https://openrouter.ai/api/v1",
		"llm.model":              "openai/gpt-4o-mini",
		"llm.api_key.openrouter": "provider-secret",
	})
	ok, detail := connectionTests(set, nil, nil)["llm"](context.Background())
	if !ok {
		t.Fatalf("configured OpenRouter reported unavailable: %s", detail)
	}
	if gotKey != "provider-secret" {
		t.Errorf("listed models with key %q, want the provider's namespaced key", gotKey)
	}
}

// #2735: AI settings showed "Connection OK" above "No models returned". A hosted provider whose
// model list cannot be read is not a working connection.
func TestLLMConnectionCheck_HostedProviderWithNoModelsIsNotOK(t *testing.T) {
	stubHostedModels(t, func(context.Context, string, string) error {
		return errors.New("the provider returned no models")
	})
	set := visionSet(t, map[string]string{
		"llm.provider":           "openai",
		"llm.hosted_provider":    "openrouter",
		"llm.url":                "https://openrouter.ai/api/v1",
		"llm.model":              "openai/gpt-4o-mini",
		"llm.api_key.openrouter": "provider-secret",
	})
	ok, detail := connectionTests(set, nil, nil)["llm"](context.Background())
	if ok {
		t.Fatal("hosted provider with no models reported OK")
	}
	if !strings.Contains(detail, "no models came back") {
		t.Errorf("detail = %q, want it to say no models came back", detail)
	}
}

// Current Health runs this probe every 30 seconds: a successful listing is reused for the same
// address and key, a failure never is, and a new key is listed at once.
func TestHostedModelCheck_ReusesOnlySuccessForTheSameKey(t *testing.T) {
	now := time.Unix(1000, 0)
	calls := 0
	fail := false
	check := &hostedModelCheck{now: func() time.Time { return now }, list: func(context.Context, string, string) error {
		calls++
		if fail {
			return errors.New("down")
		}
		return nil
	}}
	ctx := context.Background()
	_ = check.check(ctx, "http://a/v1", "k1")
	_ = check.check(ctx, "http://a/v1", "k1")
	if calls != 1 {
		t.Fatalf("calls = %d, want the second check to reuse the first success", calls)
	}
	_ = check.check(ctx, "http://a/v1", "k2")
	if calls != 2 {
		t.Fatalf("calls = %d, want a new key listed at once", calls)
	}
	now = now.Add(hostedModelReuse + time.Second)
	fail = true
	if err := check.check(ctx, "http://a/v1", "k2"); err == nil {
		t.Fatal("expired success was reused")
	}
	if err := check.check(ctx, "http://a/v1", "k2"); err == nil || calls != 4 {
		t.Fatalf("a failure was remembered (calls = %d)", calls)
	}
}

// An operator who pins the AI in the environment (LLM_PROVIDER/URL/MODEL/API_KEY) must win
// over what the picker once persisted (config-design §3: env > db). A stale
// llm.hosted_provider brand plus its namespaced llm.api_key.<brand> used to re-brand the
// selection as OpenRouter and replace the pinned key, so every call to the pinned server
// went out with the old OpenRouter key and failed 401 while the log claimed provider=openrouter.
// Not parallel: it pins the environment.
func TestResolveSelection_EnvPinnedServerBeatsStalePickerBrand(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_URL", "http://ai-server.internal:8080/v1")
	t.Setenv("LLM_MODEL", "flash-next")
	t.Setenv("LLM_API_KEY", "pinned-key")
	set := visionSet(t, map[string]string{
		"llm.hosted_provider":    "openrouter",
		"llm.api_key.openrouter": "stale-openrouter-key",
	})

	sel := resolveSelection(set)

	if sel.Provider != "openai" {
		t.Errorf("Provider = %q, want the pinned wire kind %q (the stored openrouter brand describes a different server)", sel.Provider, "openai")
	}
	if sel.APIKey != "pinned-key" {
		t.Errorf("APIKey = %q, want the env-pinned key", sel.APIKey)
	}
}

// The picker's own selection must keep working: brand + URL agree, the namespaced key applies.
func TestResolveSelection_PickerBrandStillRestoresWhenURLMatches(t *testing.T) {
	set := visionSet(t, map[string]string{
		"llm.provider":           "openai",
		"llm.url":                "https://openrouter.ai/api/v1",
		"llm.model":              "openai/gpt-4o-mini",
		"llm.hosted_provider":    "openrouter",
		"llm.api_key.openrouter": "picked-key",
	})

	sel := resolveSelection(set)

	if sel.Provider != "openrouter" || sel.APIKey != "picked-key" {
		t.Errorf("selection = %q/%q, want openrouter/picked-key", sel.Provider, sel.APIKey)
	}
}

// Production's shape: the key is pinned by env while the DB keeps a `custom` brand and an
// OpenRouter key from an earlier picker session. The pinned key wins and the brand is not applied.
func TestResolveSelection_EnvPinnedKeyBeatsStaleCustomBrandAndOpenRouterKey(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_URL", "http://ai-server.internal:8080/v1")
	t.Setenv("LLM_API_KEY", "pinned-key")
	set := visionSet(t, map[string]string{
		"llm.hosted_provider":    "custom",
		"llm.api_key.custom":     "stale-custom-key",
		"llm.api_key.openrouter": "stale-openrouter-key",
	})

	sel := resolveSelection(set)

	if sel.Provider != "openai" || sel.APIKey != "pinned-key" {
		t.Errorf("selection = %q/%q, want openai/pinned-key", sel.Provider, sel.APIKey)
	}
}
