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
		{"openai", "https://myres.openai.azure.com/openai/v1", false},
		{"openai", "https://api.deepseek.com/v1", false},
		{"openai", "https://api.mistral.ai/v1", false},
		{"openai", "https://api.fireworks.ai/inference/v1", false},
		{"openai", "https://api.x.ai/v1", false},
		{"openai", "http://llama:8080/v1", true},
		{"openai", "http://localhost:8080/v1", true},
		{"openai", "http://[::1]:8080/v1", true},
		{"openai", "http://10.0.0.5:8080/v1", true},
		{"openai", "http://172.16.3.4:8080/v1", true},
		{"openai", "http://192.168.1.20:8080/v1", true},
		{"openai", "http://169.254.1.1:8080/v1", true},
		{"openai", "http://100.100.1.2:8080/v1", true},
		{"openai", "http://100.128.0.1:8080/v1", false},
		{"openai", "http://8.8.8.8:8080/v1", false},
		{"openai", "http://gpu.internal:8080/v1", true},
		{"openai", "http://box.home.arpa/v1", true},
		{"openai", "http://mac.local:8080/v1", true},
		{"openai", "http://evil.lan.example.com/v1", false},
	} {
		p := NewOpenAIForProvider(tc.provider, tc.url, "any-model", "")
		if p.CachesPromptPrefix() != tc.want {
			t.Errorf("%s %s: CachesPromptPrefix != %v", tc.provider, tc.url, tc.want)
		}
		if got := p.selfHosted(); got != tc.want {
			t.Errorf("%s %s: selfHosted=%v, want %v", tc.provider, tc.url, got, tc.want)
		}
	}
}
