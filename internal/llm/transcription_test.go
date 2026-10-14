package llm

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func TestOpenAI_TranscribeAudioRequestsTimedSegments(t *testing.T) {
	transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: transcriptionHTTPResponse(http.StatusOK, `{"id":"stt-1","model":"openai/whisper-large-v3","language":"en","text":"Buy now. Call today.","duration":4.2,"segments":[{"start":0.1,"end":1.5,"text":" Buy now. "},{"start":1.5,"end":4.2,"text":"Call today."}],"usage":{"prompt_tokens":9,"completion_tokens":4}}`)})
	client := NewOpenAI("https://openai.invalid/v1", "chat-model", "secret")
	client.http = &http.Client{Transport: transport}
	result, err := client.TranscribeAudio(context.Background(), TranscriptionRequest{
		Model: "openai/whisper-large-v3", Audio: []byte("wav"), Format: "wav", Language: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := transport.Requests()
	if len(requests) != 1 || requests[0].URL != "https://openai.invalid/v1/audio/transcriptions" || requests[0].Header.Get("Authorization") != "Bearer secret" {
		t.Fatalf("requests = %+v", requests)
	}
	mediaType, params, err := mime.ParseMediaType(requests[0].Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q (%v)", requests[0].Header.Get("Content-Type"), err)
	}
	reader := multipart.NewReader(strings.NewReader(string(requests[0].Body)), params["boundary"])
	form, err := reader.ReadForm(1024)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = form.RemoveAll() }()
	if form.Value["model"][0] != "openai/whisper-large-v3" || form.Value["response_format"][0] != "verbose_json" {
		t.Fatalf("form values = %+v", form.Value)
	}
	if form.Value["timestamp_granularities[]"][0] != "segment" || form.Value["language"][0] != "en" {
		t.Fatalf("form values = %+v", form.Value)
	}
	files := form.File["file"]
	if len(files) != 1 || files[0].Filename != "audio.wav" {
		t.Fatalf("files = %+v", files)
	}
	audio, err := files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = audio.Close() }()
	body, err := io.ReadAll(audio)
	if err != nil || string(body) != "wav" {
		t.Fatalf("audio = %q (%v)", body, err)
	}
	if len(result.Segments) != 2 || result.Segments[0].StartMs != 100 || result.Segments[1].EndMs != 4200 {
		t.Fatalf("segments = %+v", result.Segments)
	}
	if result.Language != "en" {
		t.Fatalf("language = %q, want en", result.Language)
	}
	if result.Attribution.Tokens.Prompt != 9 || result.Attribution.Tokens.Completion != 4 || result.Attribution.GenerationID != "stt-1" {
		t.Fatalf("attribution = %+v", result.Attribution)
	}
}

func TestOpenAI_TranscribeAudioRejectsUntimedText(t *testing.T) {
	transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: transcriptionHTTPResponse(http.StatusOK, `{"text":"words, but no timing"}`)})
	client := NewOpenAI("https://openai.invalid/v1", "stt", "")
	client.http = &http.Client{Transport: transport}
	_, err := client.TranscribeAudio(context.Background(), TranscriptionRequest{Audio: []byte("wav")})
	if err == nil {
		t.Fatal("untimed transcription accepted")
	}
}

func TestOpenAI_TranscribeAudioOmitsLanguageWhenDetectionIsRequested(t *testing.T) {
	transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: transcriptionHTTPResponse(http.StatusOK,
		`{"language":"es","text":"Hola.","segments":[{"start":0,"end":1,"text":"Hola."}]}`)})
	client := NewOpenAI("https://speech.invalid/v1", "stt", "secret")
	client.http = &http.Client{Transport: transport}
	result, err := client.TranscribeAudio(context.Background(), TranscriptionRequest{Audio: []byte("wav")})
	if err != nil {
		t.Fatal(err)
	}
	request := transport.Requests()[0]
	_, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	form, err := multipart.NewReader(strings.NewReader(string(request.Body)), params["boundary"]).ReadForm(1024)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = form.RemoveAll() }()
	if _, present := form.Value["language"]; present {
		t.Fatalf("language hint = %v, want the optional field omitted so the service auto-detects", form.Value["language"])
	}
	if result.Language != "es" {
		t.Fatalf("detected language = %q, want es", result.Language)
	}
}

func TestOpenAI_TranscribeAudioReadsOpenRouterGenerationHeader(t *testing.T) {
	transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Generation-Id": []string{"gen-header"}}, Body: io.NopCloser(strings.NewReader(`{"text":"words","segments":[{"start":0,"end":1,"text":"words"}],"usage":{"cost":0.00001}}`))}})
	client := NewOpenAIForProvider("openrouter", "https://openrouter.ai/api/v1", "stt", "")
	client.http = &http.Client{Transport: transport}
	result, err := client.TranscribeAudio(context.Background(), TranscriptionRequest{Audio: []byte("wav")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Attribution.GenerationID != "gen-header" || result.Attribution.Charge == nil {
		t.Fatalf("attribution = %+v", result.Attribution)
	}
}

func transcriptionHTTPResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}
