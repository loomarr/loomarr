// Package httpx is the shared outbound HTTP client factory (design §6, §21 phase 1).
//
// Every adapter that talks to an external service builds its client here so the
// resilience defaults live in exactly one place: hard per-service timeouts, and
// jittered-backoff retries for idempotent GETs only. Writes never client-retry —
// write recovery is owned by the idempotent reconcile loops and periodic sweeps
// (§6). A down dependency degrades one feature; it never wedges the process.
//
// Callers name the remote service with a Target, which labels the outbound metrics.
// A non-2xx answer becomes a *StatusError (CheckStatus), and Classify tells a caller
// whether the failure is transient, an auth rejection, not found or permanent; the
// retry transport uses the same classification.
//
// Every transport bounds its dial (and TLS setup) and honours the environment proxy
// (HTTP_PROXY, HTTPS_PROXY, NO_PROXY; loopback is never proxied). Services on a LAN or
// tailnet are reached directly by listing them in NO_PROXY. The one deliberate
// exception is NewPublicNamedObserved: it pins each connection to a resolved public
// address (PublicDialContext), and a proxy would resolve the name itself and defeat
// that pin, so it never uses a proxy.
package httpx

import (
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/loomarr/loomarr/internal/platform/metrics"
)

// Per-service hard timeouts (§6). Verified reachable at these budgets in Phase 0.
const (
	TimeoutLibrary       = 10 * time.Second // Emby/Jellyfin
	TimeoutSeerr         = 10 * time.Second
	TimeoutTMDB          = 10 * time.Second
	TimeoutArr           = 10 * time.Second // Sonarr/Radarr
	TimeoutTunarr        = 20 * time.Second // lineup pushes are chunky
	TimeoutNotifications = 15 * time.Second
	TimeoutReference     = 10 * time.Second // operator-supplied public reference pages
	// TimeoutTunarrBulk covers the ONE bulk read the programmer makes: the
	// content-index build (GET /api/media-libraries/{id}/programs), which returns a
	// library's ENTIRE persisted program list with no server-side paging. On a large
	// library that is tens of MB and many seconds — a live homelab returned 52 MB /
	// ~17 s for 15,788 programs — so it needs far more headroom than a channel CRUD
	// call. Kept separate so the snappy 20 s ceiling still guards every other call.
	TimeoutTunarrBulk = 120 * time.Second
	TimeoutLLM        = 120 * time.Second
	TimeoutProbe      = 5 * time.Second // quick LLM-host probes (version/tags, §8.1)
)

// New returns an *http.Client with the given hard timeout. The timeout is a
// whole-request budget (connect + headers + body), which is what we want for a
// homelab service that may hang rather than refuse.
func New(timeout time.Duration) *http.Client {
	return newClient(timeout, baseTransport(), nil)
}

// NewNamed retains the standalone/test constructor without process-global metrics.
// Application composition uses NewNamedObserved with its generation Recorder.
func NewNamed(_ Target, timeout time.Duration) *http.Client {
	return newClient(timeout, baseTransport(), nil)
}

// NewLLM returns the client for OpenAI-compatible chat calls. It carries NO whole-request
// timeout on purpose: a fixed budget cannot tell a call queued behind other clients on a
// single-slot model server (healthy, slow) from a wedged one (dead). internal/platform/llm bounds each
// call with a first-token budget, an inter-chunk idle budget and an overall cap instead, all
// cancelling through the request context. recorder may be nil (standalone tools and tests).
func NewLLM(recorder *metrics.Recorder) *http.Client {
	return NewLLMNamed(TargetLLM, 0, recorder)
}

// NewNamedObserved is NewNamed bound to one application generation's Recorder.
// Production composition uses this form; NewNamed remains for standalone tools
// and tests that do not own an application generation.
func NewNamedObserved(target Target, timeout time.Duration, recorder *metrics.Recorder) *http.Client {
	if recorder == nil {
		return NewNamed(target, timeout)
	}
	return newNamedObservedClient(target, timeout, baseTransport(), recorder)
}

// NewPublicNamedObserved builds the client for operator-supplied public URLs.
// Its dialer resolves and pins a public address for the actual connection, so a
// DNS change between validation and dial cannot turn a reference fetch into an
// internal-network request. It never uses an environment proxy (see the package doc).
func NewPublicNamedObserved(target Target, timeout time.Duration, recorder *metrics.Recorder) *http.Client {
	transport := newPublicTransport(net.DefaultResolver, boundedDial)
	client := newNamedObservedClient(target, timeout, transport, recorder)
	client.CheckRedirect = publicGetRedirects
	return client
}

// NewStreaming returns an *http.Client for long streaming reads — an Ollama model
// pull runs for minutes and a multi-GB body would blow any fixed whole-request
// budget (a Client.Timeout aborts mid-body, surfacing as "context deadline
// exceeded while reading body"). So it sets NO whole-request timeout: the caller's
// context governs the stream's lifetime. Connect + TLS + response-header stages
// are still bounded, so a dead host fails fast rather than hanging forever. Use
// New (not this) for request/response RPCs — this is for streams only (§8.1 pull).
func NewStreaming() *http.Client {
	transport := baseTransport()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return newStreamingClient(transport)
}

func newPublicTransport(resolver IPLookup, dial DialContextFunc) *http.Transport {
	transport := baseTransport()
	transport.Proxy = nil // a proxy would resolve the name itself and bypass the address pin
	transport.DialContext = PublicDialContext(resolver, dial)
	transport.ResponseHeaderTimeout = 30 * time.Second
	return transport
}

func newClient(
	timeout time.Duration,
	next http.RoundTripper,
	onRetry func(metrics.OutboundRetryReason),
) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     newRetryTransport(next, onRetry),
		CheckRedirect: getOnlyRedirects,
	}
}

func newNamedObservedClient(
	target Target,
	timeout time.Duration,
	next http.RoundTripper,
	recorder *metrics.Recorder,
) *http.Client {
	label := target.Label()
	c := newClient(timeout, next, func(reason metrics.OutboundRetryReason) {
		recorder.OutboundRetried(label, reason)
	})
	c.Transport = recorder.InstrumentTransport(label, c.Transport)
	return c
}

func newStreamingClient(next http.RoundTripper) *http.Client {
	return &http.Client{
		// No whole-request Timeout on purpose — ctx cancels the stream.
		Transport:     newRetryTransport(next, nil),
		CheckRedirect: getOnlyRedirects,
	}
}

func getOnlyRedirects(_ *http.Request, via []*http.Request) error {
	if len(via) > 0 && via[0].Method != http.MethodGet {
		return http.ErrUseLastResponse
	}
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

func publicGetRedirects(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return errors.New("stopped after 3 public redirects")
	}
	port := req.URL.Port()
	standardPort := port == "" || (req.URL.Scheme == "https" && port == "443") || (req.URL.Scheme == "http" && port == "80")
	if (req.URL.Scheme != "https" && req.URL.Scheme != "http") || req.URL.User != nil || !standardPort {
		return errors.New("public redirect must use credential-free HTTP(S) on a standard port")
	}
	return nil
}
