package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAI_TranscribeAudioRequestsTimedSegments(t *testing.T) {
	var got transcriptionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"text":"Buy now. Call today.","duration":4.2,"segments":[{"start":0.1,"end":1.5,"text":" Buy now. "},{"start":1.5,"end":4.2,"text":"Call today."}]}`))
	}))
	defer srv.Close()

	client := NewOpenAI(srv.URL, "chat-model", "secret")
	segments, err := client.TranscribeAudio(context.Background(), TranscriptionRequest{
		Model: "openai/whisper-large-v3", Audio: []byte("wav"), Format: "wav", Language: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "openai/whisper-large-v3" || got.ResponseFormat != "verbose_json" {
		t.Fatalf("request = %+v", got)
	}
	if len(got.TimestampGranularities) != 1 || got.TimestampGranularities[0] != "segment" {
		t.Fatalf("timestamp granularities = %v", got.TimestampGranularities)
	}
	if got.InputAudio.Data != "d2F2" || got.InputAudio.Format != "wav" {
		t.Fatalf("audio = %+v", got.InputAudio)
	}
	if len(segments) != 2 || segments[0].StartMs != 100 || segments[1].EndMs != 4200 {
		t.Fatalf("segments = %+v", segments)
	}
}

func TestOpenAI_TranscribeAudioRejectsUntimedText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"text":"words, but no timing"}`))
	}))
	defer srv.Close()

	_, err := NewOpenAI(srv.URL, "stt", "").TranscribeAudio(context.Background(), TranscriptionRequest{Audio: []byte("wav")})
	if err == nil {
		t.Fatal("untimed transcription accepted")
	}
}
