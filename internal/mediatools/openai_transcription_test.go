package mediatools_test

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/mediatools"
	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func TestOpenAITranscriptionClientSendsDedicatedAuthenticatedMultipart(t *testing.T) {
	transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(bytes.NewBufferString(
			`{"text":"Buy now.","segments":[{"start":0.25,"end":1.5,"text":" Buy now. "}]}`,
		)),
	}})
	client := &mediatools.OpenAITranscriptionClient{
		BaseURL: func() string { return "http://speech.internal:8083/v1/" },
		APIKey:  func() string { return "dedicated-speech-secret" },
		HTTP:    &http.Client{Transport: transport},
	}

	segments, err := client.TranscribeAudio(t.Context(), "whisper-turbo", "wav", "en", []byte("wav-data"))
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 || segments[0].StartMs != 250 || segments[0].EndMs != 1500 || segments[0].Text != "Buy now." {
		t.Fatalf("segments = %+v", segments)
	}
	requests := transport.Requests()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	request := requests[0]
	if request.Method != http.MethodPost || request.URL != "http://speech.internal:8083/v1/audio/transcriptions" {
		t.Fatalf("request = %s %s", request.Method, request.URL)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer dedicated-speech-secret" {
		t.Fatalf("Authorization = %q", got)
	}
	mediaType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("Content-Type = %q, %v", request.Header.Get("Content-Type"), err)
	}
	reader := multipart.NewReader(bytes.NewReader(request.Body), parameters["boundary"])
	form, err := reader.ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = form.RemoveAll() }()
	for name, want := range map[string]string{
		"model":                     "whisper-turbo",
		"language":                  "en",
		"response_format":           "verbose_json",
		"timestamp_granularities[]": "segment",
	} {
		if got := form.Value[name]; len(got) != 1 || got[0] != want {
			t.Errorf("%s = %v, want %q", name, got, want)
		}
	}
	files := form.File["file"]
	if len(files) != 1 || files[0].Filename != "span.wav" {
		t.Fatalf("file = %+v", files)
	}
	upload, err := files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = upload.Close() }()
	if got, _ := io.ReadAll(upload); string(got) != "wav-data" {
		t.Fatalf("audio = %q", got)
	}
}

func TestOpenAITranscriptionClientRejectsUntimedTextAndMissingCredential(t *testing.T) {
	transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"text":"untimed","segments":[]}`)),
	}})
	client := &mediatools.OpenAITranscriptionClient{
		BaseURL: func() string { return "https://speech.invalid/v1" },
		APIKey:  func() string { return "secret" },
		HTTP:    &http.Client{Transport: transport},
	}
	if _, err := client.TranscribeAudio(context.Background(), "model", "wav", "", []byte("wav")); err == nil {
		t.Fatal("untimed text was accepted")
	}
	client.APIKey = func() string { return "" }
	if _, err := client.TranscribeAudio(context.Background(), "model", "wav", "", []byte("wav")); err == nil {
		t.Fatal("missing dedicated credential was accepted")
	}
}
