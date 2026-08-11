package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mantonx/loomarr/internal/config"
)

// healthcheckTimeout bounds the probe below the interval any sane orchestrator uses.
// docker/compose.yaml sets `timeout: 5s`; finishing inside that means an unhealthy
// verdict is OUR answer rather than the runtime killing the probe, which reports the
// same status for "not ready" and "the check itself hung".
const healthcheckTimeout = 3 * time.Second

// runHealthcheck probes this instance's own readiness endpoint and reports it as an
// exit status, for `HEALTHCHECK`/`healthcheck:` directives.
//
// WHY A SUBCOMMAND: the runtime image ships no HTTP client — curl is purged from the
// debian-slim base to keep the layer lean — so a compose healthcheck has nothing to
// call with. The binary is already in the image, already knows LISTEN_ADDR, and cannot
// drift from the server's own idea of where it listens.
//
// ⚠ THIS DISPATCH IS THE FIX FOR A REAL DEFECT, not an enhancement. compose.yaml
// invoked `/loomarr healthcheck` while main() inspected no os.Args whatsoever, so the
// argument was ignored and the process BOOTED A SECOND FULL LOOMARR every 30 seconds.
// It failed to bind the already-taken port and exited non-zero, so the container
// reported `unhealthy` for its entire life and `depends_on: service_healthy` never
// released. Nothing detected it because an unhealthy container still serves traffic.
//
// It probes /v1/readyz, not /healthz: liveness answers "is the process up", which the
// exec itself already proves. Readiness answers "is the store open and migrated", which
// is the question an orchestrator gating on this actually has (§17).
func runHealthcheck() error {
	cfg, err := config.Load()
	if err != nil {
		// A config the process cannot parse is a process that cannot serve, so
		// reporting unhealthy is the honest answer rather than a special case.
		return fmt.Errorf("load config: %w", err)
	}

	url := "http://" + healthcheckHostPort(cfg.ListenAddr) + "/v1/readyz"
	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	// A dedicated client, not http.DefaultClient: the default has no timeout, and the
	// context deadline alone would not bound a stalled TLS/dial phase as clearly.
	resp, err := (&http.Client{Timeout: healthcheckTimeout}).Do(req)
	if err != nil {
		return fmt.Errorf("probe %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// /v1/readyz answers 503 with a {ready, detail} body while the store is
		// unreachable or unmigrated. The status is what decides the exit code; the
		// detail is left to `docker inspect`, which captures this output.
		return fmt.Errorf("not ready: %s returned %s", url, resp.Status)
	}
	return nil
}

// healthcheckHostPort turns a LISTEN_ADDR into something dialable from inside the same
// container. A wildcard bind (":8080", "0.0.0.0:8080", "[::]:8080") is an address to
// LISTEN on, not one to CONNECT to — dialing it works on Linux by accident and is not
// a property to rely on — so the loopback address is substituted while the port is kept.
func healthcheckHostPort(listenAddr string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		// Not host:port at all (a bare port, or malformed). Treat the whole value as
		// the port, matching how net/http itself tolerates ":8080"-ish input, rather
		// than failing a healthcheck over a formatting edge case.
		return net.JoinHostPort("127.0.0.1", listenAddr)
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// errUnknownSubcommand keeps the dispatch in main() total: an unrecognised argument
// fails loudly instead of silently booting a server, which is precisely how the
// `healthcheck` defect above stayed invisible for so long.
var errUnknownSubcommand = errors.New("unknown subcommand")
