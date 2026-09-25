package llm

import "testing"

func TestOpenAI_SelfHostedIsDecidedByEndpointNotModel(t *testing.T) {
	for _, tc := range []struct {
		provider, url string
		want          bool
	}{
		{"openai", "http://llm.lan:8080/v1", true},
		{"openai", "http://127.0.0.1:8080/v1", true},
		{"openai", "https://api.openai.com/v1", false},
		{"openai", "https://generativelanguage.googleapis.com/v1beta/openai", false},
		{"openai", "https://api.groq.com/openai/v1", false},
		{"openrouter", "http://llm.lan:8080/v1", false},
		{CustomProviderKey, "https://api.example.com/v1", true},
		{"openai", "::not a url", false},
	} {
		if got := NewOpenAIForProvider(tc.provider, tc.url, "any-model", "").selfHosted(); got != tc.want {
			t.Errorf("%s %s: selfHosted=%v, want %v", tc.provider, tc.url, got, tc.want)
		}
	}
}
