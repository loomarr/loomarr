package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// The hosted picker stores credentials under the branded provider, not the flattened `openai`
// wire kind. The filler language path must resolve that same active selection or it sends an
// unauthenticated request even though Settings says the provider is configured.
func TestHostedLanguageAsker_UsesTheSelectedProvidersNamespacedKey(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"en"}}]}`))
	}))
	t.Cleanup(server.Close)

	set := visionSet(t, map[string]string{
		"llm.provider":           "openai",
		"llm.hosted_provider":    "openrouter",
		"llm.url":                server.URL,
		"llm.model":              "audio-model",
		"llm.api_key.openrouter": "provider-secret",
	})
	asker := hostedLanguageAsker(set)
	if asker == nil {
		t.Fatal("hosted language asker is nil for a configured provider")
	}
	if _, err := asker.AskAboutAudio(context.Background(), filler.AudioAsk{
		Audio: []byte("audio"), Format: "wav", Prompt: "language?",
	}); err != nil {
		t.Fatalf("ask about audio: %v", err)
	}
	if authorization != "Bearer provider-secret" {
		t.Errorf("authorization = %q, want the selected provider's namespaced key", authorization)
	}
}
