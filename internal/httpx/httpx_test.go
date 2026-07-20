package httpx

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// slowStream writes a body in chunks with a delay between each, so the whole
// response takes ~600ms — longer than a short whole-request timeout, but each
// chunk arrives promptly (headers are immediate). Mirrors an Ollama model pull:
// a long-lived body that is never idle, only large.
func slowStream(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	for i := 0; i < 5; i++ {
		if _, err := fmt.Fprintf(w, "{\"chunk\":%d}\n", i); err != nil {
			return
		}
		if fl != nil {
			fl.Flush()
		}
		time.Sleep(120 * time.Millisecond)
	}
}

// A fixed whole-request Timeout (what New gives) aborts a healthy-but-slow stream
// mid-body — the exact bug that killed the model pull at 120s. Proven here at
// 200ms so the test is fast.
func TestNew_WholeRequestTimeoutAbortsSlowStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(slowStream))
	defer srv.Close()

	resp, err := New(200 * time.Millisecond).Get(srv.URL)
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		if _, rerr := io.ReadAll(resp.Body); rerr == nil {
			t.Fatal("expected the whole-request timeout to abort the slow stream, but it completed")
		}
	}
}

// NewStreaming carries no whole-request budget, so the same slow stream reads to
// completion — cancellation is by context, not a fixed ceiling.
func TestNewStreaming_ReadsSlowStreamToCompletion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(slowStream))
	defer srv.Close()

	c := NewStreaming()
	if c.Timeout != 0 {
		t.Fatalf("NewStreaming must have no whole-request timeout, got %v", c.Timeout)
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("streaming GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read full stream: %v", err)
	}
	if got := string(b); got == "" || len(b) < 25 {
		t.Fatalf("expected all 5 chunks, got %q", got)
	}
}

// NewNamed adds outbound metrics transparently: the instrumented transport must
// not change the client's behaviour — a normal request still round-trips and
// still honours the whole-request timeout.
func TestNewNamedIsTransparent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	c := NewNamed("test-target", 5*time.Second)
	if c.Timeout != 5*time.Second {
		t.Fatalf("NewNamed timeout = %v, want 5s", c.Timeout)
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("instrumented GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "ok" {
		t.Fatalf("body = %q, want ok", string(b))
	}
}
