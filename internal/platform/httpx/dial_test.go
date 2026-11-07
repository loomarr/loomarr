package httpx

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/metrics"
)

// blockingDial stands in for the network: it never connects, it waits for the
// dial context to end, so the test observes exactly the bound the transport put on it.
func blockingDial(calls *atomic.Int32) DialContextFunc {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		calls.Add(1)
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

func useDial(t *testing.T, dial DialContextFunc, budget time.Duration) {
	t.Helper()
	prevDial, prevBudget := baseDial, dialTimeout
	baseDial, dialTimeout = dial, budget
	t.Cleanup(func() { baseDial, dialTimeout = prevDial, prevBudget })
}

// clientsUnderTest builds one client per httpx constructor. The proxy flag says
// whether the constructor must honour the environment proxy.
func clientsUnderTest() []struct {
	name  string
	proxy bool
	build func() *http.Client
} {
	rec := metrics.New(metrics.Options{})
	return []struct {
		name  string
		proxy bool
		build func() *http.Client
	}{
		{"New", true, func() *http.Client { return New(time.Minute) }},
		{"NewNamed", true, func() *http.Client { return NewNamed(TargetTMDB, time.Minute) }},
		{"NewNamedObserved", true, func() *http.Client { return NewNamedObserved(TargetTMDB, time.Minute, rec) }},
		{"NewLLM", true, func() *http.Client { return NewLLM(rec) }},
		{"NewLLMNamed", true, func() *http.Client { return NewLLMNamed(TargetLLM, time.Minute, rec) }},
		{"NewLLMNamed/nil recorder", true, func() *http.Client { return NewLLMNamed(TargetLLM, time.Minute, nil) }},
		{"NewStreaming", true, func() *http.Client { return NewStreaming() }},
		{"NewPublicNamedObserved", false, func() *http.Client { return NewPublicNamedObserved(TargetReference, time.Minute, rec) }},
	}
}

func TestEveryTransportBoundsItsDial(t *testing.T) {
	const budget = 50 * time.Millisecond
	for _, tc := range clientsUnderTest() {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			useDial(t, blockingDial(&calls), budget)

			// POST: no retry loop, so one dial attempt measures the bound itself.
			// The request context is a backstop so an unbounded dial fails the test fast.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://93.184.216.34/", nil)
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			resp, err := tc.build().Do(req)
			if err == nil {
				_ = resp.Body.Close()
				t.Fatal("dial unexpectedly succeeded")
			}
			if calls.Load() == 0 {
				t.Fatal("transport did not dial through the shared bounded dialer")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("err = %v, want a dial deadline", err)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Errorf("dial took %v, want it bounded near %v", elapsed, budget)
			}
		})
	}
}

// proxyHits counts requests that reached the stand-in proxy.
var proxyHits atomic.Int32

// TestMain installs a stand-in HTTP proxy in the process environment. net/http
// reads HTTP_PROXY a single time per process, so it must be set before any
// client in this package sends its first request. Loopback targets (every
// httptest server) are never proxied, so the other tests are unaffected.
func TestMain(m *testing.M) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, kv := range [][2]string{
		{"HTTP_PROXY", srv.URL}, {"http_proxy", srv.URL}, {"NO_PROXY", ""}, {"no_proxy", ""},
	} {
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	srv.Close()
	os.Exit(code)
}

// A proxy resolves the destination name itself, so on the public client it would
// bypass the address pin. The transport must carry no proxy at all.
func TestPublicTransportHasNoProxy(t *testing.T) {
	transport := newPublicTransport(fixedIPLookup{}, boundedDial)
	if transport.Proxy != nil {
		t.Fatal("public transport must not use an environment proxy")
	}
	if baseTransport().Proxy == nil {
		t.Fatal("base transport must honour the environment proxy")
	}
}

func TestEnvironmentProxyIsHonouredExceptOnThePublicClient(t *testing.T) {
	for _, tc := range clientsUnderTest() {
		t.Run(tc.name, func(t *testing.T) {
			before := proxyHits.Load()
			// A name no resolver can answer: reaching the proxy is the only way to succeed.
			resp, err := tc.build().Get("http://upstream.invalid/ping")
			if resp != nil {
				_ = resp.Body.Close()
			}
			viaProxy := proxyHits.Load() > before
			if tc.proxy && (!viaProxy || err != nil) {
				t.Errorf("request did not go through the environment proxy (viaProxy=%v err=%v)", viaProxy, err)
			}
			if !tc.proxy && (viaProxy || err == nil) {
				t.Errorf("public client must dial directly, never via a proxy (viaProxy=%v err=%v)", viaProxy, err)
			}
		})
	}
}
