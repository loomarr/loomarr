package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/loomarr/loomarr/internal/metrics"
)

// llmIdleConnTimeout is how long the LLM pool keeps an unused connection. It is deliberately
// shorter than the ~5 s keep-alive of the llama.cpp server (cpp-httplib's default), the
// shortest-lived server we talk to, so the pool drops a connection before the server does. A
// homelab reconnect costs a millisecond; a hosted TLS handshake ~100 ms — both noise against a
// multi-second completion. Idempotency-Key replay (below) covers the remaining race.
const llmIdleConnTimeout = 3 * time.Second

// NewLLMNamed is NewLLM with a whole-request budget, for the adapters (Ollama, video) that keep one.
func NewLLMNamed(target string, timeout time.Duration, recorder *metrics.Recorder) *http.Client {
	if recorder == nil {
		return newClient(timeout, newLLMTransport(), nil)
	}
	return newNamedObservedClient(target, timeout, newLLMTransport(), recorder)
}

func newLLMTransport() http.RoundTripper {
	return replayableWrites{next: &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     llmIdleConnTimeout,
	}}
}

// replayableWrites tags every rewindable POST with a fresh Idempotency-Key. That header is what
// lets net/http's transport transparently resend a request when the pooled keep-alive
// connection it picked was closed by the server while idle ("http: server closed idle
// connection", #1493) — otherwise a POST is never replayed and the caller sees a 1 ms failure.
//
// The replay is safe for the reason net/http chooses it: it only happens on a REUSED
// connection that failed before any response byte arrived, and never on a fresh connection, a
// timeout, or an HTTP status. The key is per request, so it never links two distinct calls.
type replayableWrites struct{ next http.RoundTripper }

func (r replayableWrites) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost || req.GetBody == nil || req.Header.Get("Idempotency-Key") != "" {
		return r.next.RoundTrip(req)
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return r.next.RoundTrip(req) // no key, no replay: the pre-#1493 behaviour, never a failure
	}
	tagged := req.Clone(req.Context())
	tagged.Header.Set("Idempotency-Key", hex.EncodeToString(raw[:]))
	return r.next.RoundTrip(tagged)
}
