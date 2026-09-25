package app

import (
	"context"
	"testing"
)

func TestLLMConnectionCheck_AcceptsSelectedProvidersNamespacedKey(t *testing.T) {
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
