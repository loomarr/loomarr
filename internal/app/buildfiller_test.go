package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestFillerSourceAdapter_HotEnablesTunarrAnnotation(t *testing.T) {
	client := testkit.NewTunarr()
	enabled := false
	adapter := fillerSourceAdapter{
		prog:       client,
		configured: func() bool { return enabled },
	}
	if got, err := adapter.LocalClipIDsByName(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("disabled annotation = %v, %v; want empty success", got, err)
	}
	if client.FillerClipReads != 0 {
		t.Fatalf("disabled adapter made %d Tunarr calls", client.FillerClipReads)
	}

	enabled = true
	if _, err := adapter.LocalClipIDsByName(context.Background()); err != nil {
		t.Fatalf("enabled annotation: %v", err)
	}
	if client.FillerClipReads != 1 {
		t.Fatalf("enabled adapter made %d calls, want 1", client.FillerClipReads)
	}
}

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

func TestHostedTranscriber_UsesTheSelectedProvidersNamespacedKey(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"Buy now.","duration":1,"segments":[{"start":0,"end":1,"text":"Buy now."}]}`))
	}))
	t.Cleanup(server.Close)

	// The seam under test is provider selection, not ffmpeg. This stand-in writes the requested
	// output path so the production HostedTranscriber reaches its HTTP client without requiring a
	// media fixture or weakening its extraction contract.
	ffmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nfor last; do :; done\nprintf wav > \"$last\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	set := visionSet(t, map[string]string{
		"playout.ffmpeg_path":        ffmpeg,
		"llm.provider":               "openai",
		"llm.hosted_provider":        "openrouter",
		"llm.url":                    server.URL,
		"llm.model":                  "openai/gpt-4o-mini",
		"llm.api_key.openrouter":     "provider-secret",
		"filler.transcribe.provider": "hosted",
		"filler.transcribe.model":    "openai/whisper-large-v3",
	})

	segments, err := buildFillerMediaTools(set).Transcribe(context.Background(), "clip.mp4", 0, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].Text != "Buy now." {
		t.Fatalf("segments = %+v", segments)
	}
	if authorization != "Bearer provider-secret" {
		t.Errorf("authorization = %q, want the selected provider's namespaced key", authorization)
	}
}

func TestFillerTagger_UsesTheSelectedProvidersNamespacedKey(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	t.Cleanup(server.Close)

	set := visionSet(t, map[string]string{
		"filler.ai_tagging":      "true",
		"llm.provider":           "openai",
		"llm.hosted_provider":    "openrouter",
		"llm.url":                server.URL,
		"llm.model":              "openai/gpt-4o-mini",
		"llm.api_key.openrouter": "provider-secret",
	})
	provider, _ := buildTagger(nil, set, nil)
	if provider == nil {
		t.Fatal("tagger provider is nil for configured OpenRouter")
	}
	if _, err := provider.Chat(context.Background(), nil, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer provider-secret" {
		t.Errorf("authorization = %q, want the selected provider's namespaced key", authorization)
	}
}
