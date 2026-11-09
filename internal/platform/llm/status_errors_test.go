package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/httpx"
)

func replyServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// requireStatus asserts err is (or wraps) an httpx.StatusError with the status and has exactly
// the text an operator already reads.
func requireStatus(t *testing.T, err error, status int, class httpx.Class, text string) {
	t.Helper()
	var se *httpx.StatusError
	if !errors.As(err, &se) || se.Status != status {
		t.Fatalf("error = %v, want an *httpx.StatusError with status %d", err, status)
	}
	if got := httpx.Classify(err); got != class {
		t.Errorf("Classify = %v, want %v", got, class)
	}
	if err.Error() != text {
		t.Errorf("error text = %q, want %q", err.Error(), text)
	}
}

// Every remaining non-2xx path in the package is an httpx.StatusError with its text unchanged
// (status only, no provider body, except where the body was already part of the text).
func TestNon2xxPathsAreHTTPXStatusErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const body = `{"error":{"message":"private detail"}}`

	t.Run("models listing", func(t *testing.T) {
		t.Parallel()
		_, err := fetchModels(ctx, replyServer(t, http.StatusForbidden, body).URL, "k", false)
		requireStatus(t, err, http.StatusForbidden, httpx.ClassAuth, "models: status 403")
	})
	t.Run("hugging face", func(t *testing.T) {
		t.Parallel()
		err := hfGetJSON(ctx, replyServer(t, http.StatusNotFound, body).URL, &struct{}{})
		requireStatus(t, err, http.StatusNotFound, httpx.ClassNotFound, "hugging face: status 404")
	})
	t.Run("ollama chat", func(t *testing.T) {
		t.Parallel()
		_, err := NewOllama(replyServer(t, http.StatusInternalServerError, body).URL, "m").
			Chat(ctx, []Message{{Role: User, Content: "hi"}}, ChatOptions{})
		requireStatus(t, err, http.StatusInternalServerError, httpx.ClassTransient, "ollama chat: status 500")
	})
	t.Run("ollama vision", func(t *testing.T) {
		t.Parallel()
		_, err := NewOllama(replyServer(t, http.StatusBadRequest, body).URL, "m").
			AskAboutImages(ctx, "what", [][]byte{{1}})
		requireStatus(t, err, http.StatusBadRequest, httpx.ClassPermanent, "ollama vision: status 400")
	})
	t.Run("ollama warm", func(t *testing.T) {
		t.Parallel()
		err := NewOllama(replyServer(t, http.StatusBadRequest, body).URL, "m").Warm(ctx)
		requireStatus(t, err, http.StatusBadRequest, httpx.ClassPermanent, "ollama warm: status 400")
	})
	t.Run("ollama ps", func(t *testing.T) {
		t.Parallel()
		_, err := NewOllama(replyServer(t, http.StatusForbidden, body).URL, "m").ListResident(ctx)
		requireStatus(t, err, http.StatusForbidden, httpx.ClassAuth, "ollama ps: status 403")
	})
	t.Run("ollama pull", func(t *testing.T) {
		t.Parallel()
		err := NewProber(replyServer(t, http.StatusNotFound, body).URL).Pull(ctx, "m:1", nil)
		requireStatus(t, err, http.StatusNotFound, httpx.ClassNotFound, "ollama pull: status 404")
	})
	t.Run("transcription keeps the provider body in its text", func(t *testing.T) {
		t.Parallel()
		_, err := NewOpenAI(replyServer(t, http.StatusBadRequest, body).URL, "m", "").
			TranscribeAudio(ctx, TranscriptionRequest{Model: "m", Audio: []byte{1}, Format: "wav"})
		requireStatus(t, err, http.StatusBadRequest, httpx.ClassPermanent, "audio transcription: status 400: "+body)
		if got, want := SafeCause(err), "audio transcription: status 400: private detail"; got != want {
			t.Errorf("SafeCause = %q, want %q", got, want)
		}
	})
}

// ValidateKey's operator-facing sentences are unchanged, and the failure stays machine-readable.
func TestValidateKeyStatusErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, tc := range []struct {
		status int
		body   string
		text   string
		class  httpx.Class
	}{
		{http.StatusUnauthorized, "", "key rejected (401) — check the API key", httpx.ClassAuth},
		{http.StatusForbidden, "", "key rejected (403) — check the API key", httpx.ClassAuth},
		{http.StatusInternalServerError, `{"error":{"message":"overloaded"}}`, "provider returned 500: overloaded", httpx.ClassTransient},
		{http.StatusNotFound, "", "provider returned 404", httpx.ClassNotFound},
	} {
		err := ValidateKey(ctx, replyServer(t, tc.status, tc.body).URL, "k")
		requireStatus(t, err, tc.status, tc.class, tc.text)
	}
	// A 400 still means the key was accepted.
	if err := ValidateKey(ctx, replyServer(t, http.StatusBadRequest, "").URL, "k"); err != nil {
		t.Errorf("ValidateKey on 400 = %v, want nil", err)
	}
}
