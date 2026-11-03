package llm_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/httpx"
	"github.com/loomarr/loomarr/internal/llm"
)

// A non-2xx provider reply is an httpx.StatusError (status machine-readable, classified by
// httpx), while the text an operator reads and the cause persisted on a job keep the
// "<op>: status N: ..." shape.
func TestChatNon2xxIsHTTPXStatusError(t *testing.T) {
	t.Parallel()
	const body = `{"error":{"message":"model not found"}}`
	for _, tc := range []struct {
		status int
		class  httpx.Class
	}{
		{http.StatusBadRequest, httpx.ClassPermanent},
		{http.StatusUnauthorized, httpx.ClassAuth},
		{http.StatusNotFound, httpx.ClassNotFound},
		{http.StatusTooManyRequests, httpx.ClassTransient},
		{http.StatusInternalServerError, httpx.ClassTransient},
		{http.StatusNotImplemented, httpx.ClassPermanent},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			o := llm.NewOpenAI(srv.URL, "m", "")
			_, err := o.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{})

			var se *httpx.StatusError
			if !errors.As(err, &se) || se.Status != tc.status {
				t.Fatalf("Chat error = %v, want *httpx.StatusError with status %d", err, tc.status)
			}
			if got := httpx.Classify(err); got != tc.class {
				t.Errorf("Classify = %v, want %v", got, tc.class)
			}
			if want := fmt.Sprintf("openai chat: status %d: %s", tc.status, body); err.Error() != want {
				t.Errorf("error text = %q, want %q", err.Error(), want)
			}
			if want := fmt.Sprintf("openai chat: status %d: model not found", tc.status); llm.SafeCause(err) != want {
				t.Errorf("SafeCause = %q, want %q", llm.SafeCause(err), want)
			}
		})
	}
}
