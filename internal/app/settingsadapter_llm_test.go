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
	ok, detail := connectionTests(set, nil)["llm"](context.Background())
	if !ok {
		t.Fatalf("configured OpenRouter reported unavailable: %s", detail)
	}
}
