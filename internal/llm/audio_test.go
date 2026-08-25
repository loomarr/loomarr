package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAI_AskAboutAudioReturnsAttribution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-OpenRouter-Metadata"); got != "enabled" {
			t.Errorf("metadata header = %q", got)
		}
		_, _ = w.Write([]byte(`{"id":"audio-1","model":"google/gemini-audio","choices":[{"message":{"content":"en"}}],"openrouter_metadata":{"attempt":1,"endpoints":{"available":[{"provider":"Google","selected":true}]}},"usage":{"prompt_tokens":17,"completion_tokens":1,"prompt_tokens_details":{"audio_tokens":12},"cost":0.0007}}`))
	}))
	defer srv.Close()

	resp, err := NewOpenAIForProvider("openrouter", srv.URL, "google/gemini-audio", "secret").AskAboutAudio(
		context.Background(), AudioRequest{Audio: []byte("wav"), Prompt: "language?"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "en" || resp.Attribution.Tokens.Audio != 12 || resp.Attribution.ResolvedProvider != "Google" {
		t.Fatalf("response = %+v", resp)
	}
}
