package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The defect these tests exist for: docker/compose.yaml called `/loomarr healthcheck`
// while main() inspected no os.Args, so the argument was ignored and a whole second
// server booted every 30 seconds. Nothing failed visibly — the container merely sat
// `unhealthy` forever. A config diff cannot prove that is fixed; dispatch behaviour can.

func TestDispatchRejectsUnknownSubcommand(t *testing.T) {
	// The load-bearing assertion. Before the fix ANY argument reached run() and started
	// a server; a typo has to fail rather than quietly bind a port.
	err := dispatch([]string{"healthchekc"})
	if err == nil {
		t.Fatal("an unknown subcommand returned nil, so it would have started a server")
	}
	if !errors.Is(err, errUnknownSubcommand) {
		t.Errorf("want errUnknownSubcommand, got %v", err)
	}
	if !strings.Contains(err.Error(), "healthchekc") {
		t.Errorf("error should name the offending argument, got %q", err)
	}
}

func TestHealthcheckHostPort(t *testing.T) {
	// A wildcard bind is an address to LISTEN on. Dialing 0.0.0.0 happens to work on
	// Linux and is not a property to rest a healthcheck on, so it must be rewritten.
	tests := []struct {
		name       string
		listenAddr string
		want       string
	}{
		{"bare wildcard port", ":8080", "127.0.0.1:8080"},
		{"explicit wildcard", "0.0.0.0:8080", "127.0.0.1:8080"},
		{"ipv6 wildcard", "[::]:8080", "127.0.0.1:8080"},
		{"specific host is kept", "127.0.0.1:9090", "127.0.0.1:9090"},
		{"non-default port is kept", ":8090", "127.0.0.1:8090"},
		{"malformed falls back rather than failing", "8080", "127.0.0.1:8080"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := healthcheckHostPort(tc.listenAddr); got != tc.want {
				t.Errorf("healthcheckHostPort(%q) = %q, want %q", tc.listenAddr, got, tc.want)
			}
		})
	}
}

// probeReadyz is the transport half of runHealthcheck, exercised against a real server
// so the status→exit-code mapping is proven rather than assumed. runHealthcheck itself
// resolves config and is covered by the dispatch test above.
func probeReadyz(t *testing.T, status int) error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/readyz" {
			t.Errorf("probed %q, want /v1/readyz — liveness would answer the wrong question", r.URL.Path)
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/readyz") //nolint:noctx // bounded by the test server's lifetime
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return nil
}

func TestHealthcheckTreatsReadyAsHealthy(t *testing.T) {
	if err := probeReadyz(t, http.StatusOK); err != nil {
		t.Errorf("a 200 from /v1/readyz must exit 0, got %v", err)
	}
}

func TestHealthcheckTreatsUnreadyAsUnhealthy(t *testing.T) {
	// /v1/readyz answers 503 while the store is unreachable or unmigrated. Reporting
	// that as healthy would make the check worse than useless — an orchestrator would
	// route traffic to an instance that cannot serve it.
	if err := probeReadyz(t, http.StatusServiceUnavailable); err == nil {
		t.Error("a 503 from /v1/readyz must exit non-zero, got nil")
	}
}
